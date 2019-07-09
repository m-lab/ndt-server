// Package sender implements the upload sender.
package sender

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

// TODO(bassosimone): share this function rather than duplicating
func startclosing(conn *websocket.Conn) {
	msg := websocket.FormatCloseMessage(
		websocket.CloseNormalClosure, "Done sending")
	d := time.Now().Add(spec.DefaultRuntime) // Liveness!
	err := conn.WriteControl(websocket.CloseMessage, msg, d)
	if err != nil {
		logging.Logger.WithError(err).Warn("sender: conn.WriteControl failed")
		return
	}
	logging.Logger.Debug("sender: sending Close message")
}

func loop(
	conn *websocket.Conn, src <-chan model.Measurement,
	dst chan<- model.Measurement,
) {
	logging.Logger.Debug("sender: start")
	defer logging.Logger.Debug("sender: stop")
	defer close(dst)
	defer func() {
		for range src {
			// make sure we drain the channel
		}
	}()
	for {
		m, ok := <-src
		if !ok { // This means that the previous step has terminated
			startclosing(conn)
			return
		}
		conn.SetWriteDeadline(time.Now().Add(spec.DefaultRuntime)) // Liveness!
		if err := conn.WriteJSON(m); err != nil {
			logging.Logger.WithError(err).Warn("sender: conn.WriteJSON failed")
			return
		}
		dst <- m // Liveness: this is blocking
	}
}

// Start starts the sender in a background goroutine. The sender will send
// to the client the measurement messages coming from |src|. These messages
// will also be emitted to the returned channel.
//
// Liveness guarantee: the sender will not be stuck sending for more then
// the DefaultRuntime of the subtest, provided that the consumer will
// continue reading from the returned channel.
func Start(
	conn *websocket.Conn, src <-chan model.Measurement,
) <-chan model.Measurement {
	dst := make(chan model.Measurement)
	go loop(conn, src, dst)
	return dst
}
