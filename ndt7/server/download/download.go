// Package download implements the ndt7/server downloader.
package download

import (
	"context"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/server/download/measurer"
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

// makePreparedMessage generates a prepared message that should be sent
// over the network for generating network load.
func makePreparedMessage(size int) (*websocket.PreparedMessage, error) {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	data := make([]byte, size)
	// This is not the fastest algorithm to generate a random string, yet it
	// is most likely good enough for our purposes. See [1] for a comprehensive
	// discussion regarding how to generate a random string in Golang.
	//
	// .. [1] https://stackoverflow.com/a/31832326/4354461
	for i := range data {
		data[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return websocket.NewPreparedMessage(websocket.BinaryMessage, data)
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
	logging.Logger.Debug("Generating random buffer")
	const bulkMessageSize = 1 << 13
	preparedMessage, err := makePreparedMessage(bulkMessageSize)
	if err != nil {
		logging.Logger.WithError(err).Warn("Cannot prepare message")
		return
	}
	ctx, cancel := context.WithCancel(request.Context())
	measurements := measurer.Start(ctx, request, conn, dl.DataDir)
	logging.Logger.Debug("Start sending data to client")
	conn.SetReadLimit(spec.MinMaxMessageSize)
	// make sure we cleanup resources when we leave
	defer func() {
		logging.Logger.Debug("Closing the WebSocket connection")
		conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(
			websocket.CloseNormalClosure, ""), time.Now().Add(defaultTimeout))
		// We could leave the context because the measuring goroutine thinks we're
		// done or because there has been a socket error. In the latter case, it is
		// important to synchronise with the goroutine and wait for it to exit.
		cancel()
		for range measurements {
			// NOTHING
		}
	}()
	for {
		select {
		case m, ok := <-measurements:
			if !ok {
				return // the goroutine told us it's time to stop running
			}
			conn.SetWriteDeadline(time.Now().Add(defaultTimeout))
			if err := conn.WriteJSON(m); err != nil {
				logging.Logger.WithError(err).Warn("Cannot send measurement message")
				return
			}
		default:
			conn.SetWriteDeadline(time.Now().Add(defaultTimeout))
			if err := conn.WritePreparedMessage(preparedMessage); err != nil {
				logging.Logger.WithError(err).Warn("Cannot send prepared message")
				return
			}
		}
	}
}
