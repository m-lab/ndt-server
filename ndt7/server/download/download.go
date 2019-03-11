// Package download implements the ndt7/server downloader.
package download

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/server/download/measurer"
	"github.com/m-lab/ndt-server/ndt7/server/download/sender"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

// defaultTimeout is the default value of the I/O timeout.
const defaultTimeout = 7 * time.Second

// defaultDuration is the default duration of a subtest in nanoseconds.
const defaultDuration = 10 * time.Second

// Handler handles a download subtest from the server side.
type Handler struct {
	Upgrader websocket.Upgrader
	DataDir  string
}

// warnAndClose emits a warning |message| and then closes the HTTP connection
// using the |writer| http.ResponseWriter.
func warnAndClose(writer http.ResponseWriter, message string) {
	logging.Logger.Warn(message)
	writer.Header().Set("Connection", "Close")
	writer.WriteHeader(http.StatusBadRequest)
}

// Handle handles the download subtest.
func (dl Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	logging.Logger.Debug("Upgrading to WebSockets")
	if request.Header.Get("Sec-WebSocket-Protocol") != spec.SecWebSocketProtocol {
		warnAndClose(writer, "Missing Sec-WebSocket-Protocol in request")
		return
	}
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	conn, err := dl.Upgrader.Upgrade(writer, request, headers)
	if err != nil {
		warnAndClose(writer, "Cannnot UPGRADE to WebSocket")
		return
	}
	// TODO(bassosimone): an error before this point means that the *os.File
	// will stay in cache until the cache pruning mechanism is triggered. This
	// should be a small amount of seconds. If Golang does not call shutdown(2)
	// and close(2), we'll end up keeping sockets that caused an error in the
	// code above (e.g. because the handshake was not okay) alive for the time
	// in which the corresponding *os.File is kept in cache.
	defer conn.Close()
	ctx, cancel := context.WithTimeout(request.Context(), defaultDuration)
	defer cancel()
	err = sender.Start(conn, measurer.Start(ctx, request, conn, dl.DataDir))
	if err != nil {
		logging.Logger.WithError(err).Warn("the download pipeline failed")
		return
	}
	err = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(
			websocket.CloseNormalClosure, ""), time.Now().Add(defaultTimeout))
	if err != nil {
		logging.Logger.WithError(err).Warn("cannot send the CLOSE message")
		return
	}
}
