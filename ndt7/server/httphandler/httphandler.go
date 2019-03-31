// Package httphandler contains the ndt7 HTTP handler.
//
// The handler handles the ndt7 URLs:
//
// - /ndt/v7/download
// - /ndt/v7/upload
//
// The handler performs all the common initialization required by
// both subtests and manages the resources. It will dispatch control
// to the proper subtest depending on the URL path.
package httphandler

import (
	"context"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/server/download"
	"github.com/m-lab/ndt-server/ndt7/server/results"
	"github.com/m-lab/ndt-server/ndt7/server/upload"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

// Handler is the context for running ndt7 subtests
type Handler struct {
	// Upgrader is the websocket upgrader
	Upgrader websocket.Upgrader

	// DataDir is the directory where to save data
	DataDir  string
}

// warnAndClose emits a warning and closes the connection with 400.
func warnAndClose(writer http.ResponseWriter, message string) {
	logging.Logger.Warn(message)
	writer.Header().Set("Connection", "Close")
	writer.WriteHeader(http.StatusBadRequest)
}

// ServeHTTP upgrades the connection to WebSocket, opens the results file, and
// dispatches control to the proper subtest handler. This function will keep
// ownership of the websocket connection and of the results file and will make
// sure they are closed, using defer, when this function returns.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var fn func(context.Context, *websocket.Conn, *results.File)
	var dirname string
	if r.URL.Path == spec.DownloadURLPath {
		dirname = "download"
		fn = download.Do
	} else if r.URL.Path == spec.UploadURLPath {
		dirname = "upload"
		fn = upload.Do
	} else {
		warnAndClose(w, "httphandler: ndt7 URL paths incorrectly configured")
		return
	}
	logging.Logger.Debug("httphandler: upgrading to WebSockets")
	if r.Header.Get("Sec-WebSocket-Protocol") != spec.SecWebSocketProtocol {
		warnAndClose(w, "httphandler: invalid Sec-WebSocket-Protocol header")
		return
	}
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	conn, err := h.Upgrader.Upgrade(w, r, headers)
	if err != nil {
		warnAndClose(w, "httphandler: cannot UPGRADE to WebSocket")
		return
	}
	// TODO(bassosimone): an error before this point means that the *os.File
	// will stay in cache until the cache pruning mechanism is triggered. This
	// should be a small amount of seconds. If Golang does not call shutdown(2)
	// and close(2), we'll end up keeping sockets that caused an error in the
	// code above (e.g. because the handshake was not okay) alive for the time
	// in which the corresponding *os.File is kept in cache.
	defer warnonerror.Close(conn, "httphandler: ignoring conn.Close result")
	resultfp, err := results.OpenFor(r, conn, h.DataDir, dirname)
	if err != nil {
		return // error already printed
	}
	defer warnonerror.Close(
		resultfp, "httphandler: ignoring resultfp.Close result",
	)
	// Implementation note: use child context so that, if we cannot save the
	// results in the loop below, we terminate the goroutines early
	wholectx, cancel := context.WithCancel(r.Context())
	defer cancel()
	fn(wholectx, conn, resultfp)
}
