// Package sender implements the download sender.
package sender

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/closer"
	"github.com/m-lab/ndt-server/ndt7/download/sender/binary"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

func loop(conn *websocket.Conn, src <-chan model.Measurement, dst chan<- model.Measurement) {
	logging.Logger.Debug("sender: start")
	defer logging.Logger.Debug("sender: stop")
	defer close(dst)
	defer func() {
		for range src {
			// make sure we drain the channel
		}
	}()
	bm := binary.NewMessage()
	for {
		select {
		case meas, ok := <-src:
			if !ok { // This means that the previous step has terminated
				closer.StartClosing(conn)
				return
			}
			conn.SetWriteDeadline(time.Now().Add(spec.DefaultRuntime)) // Liveness!
			if err := conn.WriteJSON(meas); err != nil {
				logging.Logger.WithError(err).Warn("sender: conn.WriteJSON failed")
				return
			}
			dst <- meas // Liveness: this is blocking
		default:
			conn.SetWriteDeadline(time.Now().Add(spec.DefaultRuntime)) // Liveness!
			if err := bm.Send(conn); err != nil {
				logging.Logger.WithError(err).Warn("sender: m.Send(conn) failed")
				return
			}
		}
	}
}

// Start starts the sender in a background goroutine. The sender will send
// binary messages and measurement messages coming from |src|. Such messages
// will also be emitted to the returned channel.
//
// Liveness guarantee: the sender will not be stuck sending for more then
// the DefaultRuntime of the subtest, provided that the consumer will
// continue reading from the returned channel.
func Start(conn *websocket.Conn, src <-chan model.Measurement) <-chan model.Measurement {
	dst := make(chan model.Measurement)
	go loop(conn, src, dst)
	return dst
}
