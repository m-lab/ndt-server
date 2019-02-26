package upload

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

const (
	// defaultTimeout is the default value of the I/O timeout.
	defaultTimeout = 7 * time.Second

	// defaultDuration is the default duration of a subtest in nanoseconds.
	defaultDuration = 10 * time.Second
)

// Handler handles an upload test on the server side.
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

// startMeasuring runs the measurement loop. This runs in a separate goroutine
// and emits Measurement events on the returned channel.
func startMeasuring(ctx context.Context, request *http.Request, conn *websocket.Conn, dataDir string) chan model.Measurement {
	dst := make(chan model.Measurement)
	//go measuringLoop(ctx, request, conn, dataDir, dst)
	return dst
}

// ServeHTTP handles the upload subtest.
func (ul Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// TODO: factor out this stuff as it's the same for both UL/DL
	logging.Logger.Debug("Upgrading to WebSockets")

	if request.Header.Get("Sec-WebSocket-Protocol") != spec.SecWebSocketProtocol {
		warnAndClose(writer, "Missing Sec-WebSocket-Protocol in request")
		return
	}
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)

	conn, err := ul.Upgrader.Upgrade(writer, request, headers)
	if err != nil {
		warnAndClose(writer, "Cannot UPGRADE to WebSocket")
		return
	}

	defer conn.Close()

	// Read limit is set to the smallest allowed payload size.
	conn.SetReadLimit(spec.MinMaxMessageSize)

	// Start measuring loop
	logging.Logger.Debug("Starting measuring goroutine")
	ctx, cancel := context.WithCancel(request.Context())
	measurements := startMeasuring(ctx, request, conn, ul.DataDir)

	// Make sure we clean up resources
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

	logging.Logger.Debug("Starting receiving data from the client")
	for {
		mt, message, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				logging.Logger.WithError(err)
			}
			logging.Logger.Debug(request.RemoteAddr + ": connection closed.")
			break
		}

		if mt == websocket.TextMessage {
			var mdata model.Measurement
			err := json.Unmarshal(message, &mdata)
			if err == nil {
				logging.Logger.Errorf("Unable to unmarshal JSON message: %s", message)
			}
			logging.Logger.Debugf("Received Measurement - AppInfo.NumBytes: %d", mdata.AppInfo.NumBytes)
		}
	}
}
