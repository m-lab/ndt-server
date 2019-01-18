package upload

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

type Handler struct {
	Upgrader websocket.Upgrader
}

// warnAndClose emits a warning |message| and then closes the HTTP connection
// using the |writer| http.ResponseWriter.
func warnAndClose(writer http.ResponseWriter, message string) {
	logging.Logger.Warn(message)
	writer.Header().Set("Connection", "Close")
	writer.WriteHeader(http.StatusBadRequest)
}

func (ul Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// TODO: factor out this stuff as it's the same for both UL/DL
	logging.Logger.Debug("Upgrading to WebSockets")

	fmt.Printf("%s\n", request.Header)

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
			logging.Logger.Debugf("Received Measurement - AppInfo.NumBytes: %f", mdata.AppInfo.NumBytes)
		}

	}
}
