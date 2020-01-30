// Package sender implements the upload sender.
package sender

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/closer"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/ping/message"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

func loop(
	conn *websocket.Conn, src <-chan model.Measurement,
	dst chan<- model.Measurement, start time.Time,
) {
	logging.Logger.Debug("sender: start")
	defer logging.Logger.Debug("sender: stop")
	defer close(dst)
	defer func() {
		for range src {
			// make sure we drain the channel
		}
	}()
	deadline := start.Add(spec.MaxRuntime)
	err := conn.SetWriteDeadline(deadline) // Liveness!
	if err != nil {
		logging.Logger.WithError(err).Warn("sender: conn.SetWriteDeadline failed")
		return
	}
	// There is no "special" first RTT sample for /upload. The server can't control if the client
	// has already started flooding the connection.  So, if server sends ping frame here,
	// the pong frame may be still affected by HOL, like every other ping-pong frame.
	for {
		m, ok := <-src
		if !ok { // This means that the previous step has terminated
			closer.StartClosing(conn)
			return
		}
		if err := conn.WriteJSON(m); err != nil {
			logging.Logger.WithError(err).Warn("sender: conn.WriteJSON failed")
			return
		}
		dst <- m // Liveness: this is blocking
		if err := message.SendTicks(conn, start, deadline); err != nil {
			logging.Logger.WithError(err).Warn("sender: ping.message.SendTicks failed")
			return
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
func Start(conn *websocket.Conn, src <-chan model.Measurement, start time.Time) <-chan model.Measurement {
	dst := make(chan model.Measurement)
	go loop(conn, src, dst, start)
	return dst
}
