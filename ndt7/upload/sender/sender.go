// Package sender implements the upload sender.
package sender

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/closer"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/ping"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

func loop(
	conn *websocket.Conn, src <-chan model.Measurement,
	dst chan<- model.Measurement, start time.Time, pongch <-chan model.WSPingInfo,
) {
	logging.Logger.Debug("sender: start")
	defer logging.Logger.Debug("sender: stop")
	defer close(dst)
	defer func() {
		for range src {
			// make sure we drain the channel
		}
		for range pongch {
			// it should be buffered channel, but let's drain it anyway
		}
	}()
	deadline := start.Add(spec.MaxRuntime)
	err := conn.SetWriteDeadline(deadline) // Liveness!
	if err != nil {
		logging.Logger.WithError(err).Warn("sender: conn.SetWriteDeadline failed")
		return
	}
	for {
		select {
		case m, ok := <-src:
			if !ok { // This means that the previous step has terminated
				closer.StartClosing(conn)
				return
			}
			if err := conn.WriteJSON(m); err != nil {
				logging.Logger.WithError(err).Warn("sender: conn.WriteJSON failed")
				return
			}
			dst <- m // Liveness: this is blocking
			if err := ping.SendTicks(conn, start, deadline); err != nil {
				logging.Logger.WithError(err).Warn("sender: ping.SendTicks failed")
				return
			}
		case wsinfo := <-pongch:
			m := model.Measurement{
				WSPingInfo: &wsinfo,
			}
			if err := conn.WriteJSON(m); err != nil {
				logging.Logger.WithError(err).Warn("sender: conn.WriteJSON failed")
				return
			}
			dst <- m // Liveness: this is blocking write to log
		}
	}
}

// Start starts the sender in a background goroutine. The sender will send
// to the client the measurement messages coming from |src|. These messages
// will also be emitted to the returned channel.
//
// Liveness guarantee: the sender will not be stuck sending for more then
// the MaxRuntime of the subtest, provided that the consumer will
// continue reading from the returned channel. This is enforced by
// setting the write deadline to |start| + MaxRuntime.
func Start(conn *websocket.Conn, src <-chan model.Measurement, start time.Time, pongch <-chan model.WSPingInfo) <-chan model.Measurement {
	dst := make(chan model.Measurement)
	go loop(conn, src, dst, start, pongch)
	return dst
}
