// Package closer implements the WebSocket closer.
package closer

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
)

// StartClosing will start closing the websocket connection.
func StartClosing(conn *websocket.Conn) {
	msg := websocket.FormatCloseMessage(
		websocket.CloseNormalClosure, "Done sending")
	d := time.Now().Add(time.Second) // Liveness!
	err := conn.WriteControl(websocket.CloseMessage, msg, d)
	if err != nil {
		logging.Logger.WithError(err).Warn("sender: conn.WriteControl failed")
		return
	}
	logging.Logger.Debug("sender: sending Close message")
}
