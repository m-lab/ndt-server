// Package handler implements the WebSocket handler for ndt7.
package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/download"
	"github.com/m-lab/ndt-server/ndt7/results"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/ndt7/upload"
)

// Handler handles ndt7 subtests.
type Handler struct {
	// Upgrader is the WebSocket upgrader.
	Upgrader websocket.Upgrader

	// DataDir is the directory where results are saved.
	DataDir  string
}

// warnAndClose emits message as a warning and the sends a Bad Request
// response to the client using writer.
func warnAndClose(writer http.ResponseWriter, message string) {
	logging.Logger.Warn(message)
	writer.Header().Set("Connection", "Close")
	writer.WriteHeader(http.StatusBadRequest)
}

// testerFunc is the function implementing a subtest. The first argument
// is the subtest context. The second argument is the connected websocket. The
// third argument is the open file where to write results. This function does
// not own the second or the third argument.
type testerFunc = func(context.Context, *websocket.Conn, *results.File)

// downloadOrUpload implements both download and upload. The writer argument
// is the HTTP response writer. The request argument is the HTTP request
// that we received. The kind argument must be spec.SubtestDownload or
// spec.SubtestUpload. The tester is a function actually implementing the
// requested ndt7 subtest.
func (h Handler) downloadOrUpload(writer http.ResponseWriter, request *http.Request, kind spec.SubtestKind, tester testerFunc) {
	logging.Logger.Debug("downloadOrUpload: upgrading to WebSockets")
	if request.Header.Get("Sec-WebSocket-Protocol") != spec.SecWebSocketProtocol {
		warnAndClose(
			writer, "downloadOrUpload: missing Sec-WebSocket-Protocol in request")
		return
	}
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	conn, err := h.Upgrader.Upgrade(writer, request, headers)
	if err != nil {
		warnAndClose(writer, fmt.Sprintf(
			"downloadOrUpload: cannnot UPGRADE to WebSocket: %s", err,
		))
		return
	}
	// TODO(bassosimone): an error before this point means that the *os.File
	// will stay in cache until the cache pruning mechanism is triggered. This
	// should be a small amount of seconds. If Golang does not call shutdown(2)
	// and close(2), we'll end up keeping sockets that caused an error in the
	// code above (e.g. because the handshake was not okay) alive for the time
	// in which the corresponding *os.File is kept in cache.
	defer warnonerror.Close(conn, "download: ignoring conn.Close result")
	logging.Logger.Debug("downloadOrUpload: opening results file")
	resultfp, err := results.OpenFor(request, conn, h.DataDir, kind)
	if err != nil {
		return // error already printed
	}
	defer warnonerror.Close(resultfp, "download: ignoring resultfp.Close result")
	tester(request.Context(), conn, resultfp)
}

// Download handles the download subtest.
func (h Handler) Download(writer http.ResponseWriter, request *http.Request) {
	h.downloadOrUpload(writer, request, spec.SubtestDownload, download.Do)
}

// Upload handles the upload subtest.
func (h Handler) Upload(writer http.ResponseWriter, request *http.Request) {
	h.downloadOrUpload(writer, request, spec.SubtestUpload, upload.Do)
}
