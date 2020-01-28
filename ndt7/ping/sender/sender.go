// Package sender implements the pingupload sender.
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

func loop(conn *websocket.Conn, senderch <-chan model.Measurement, start time.Time) {
	logging.Logger.Debug("sender: start")
	defer logging.Logger.Debug("sender: stop")

	defer func() {
		for range senderch {
			// drain the channel (in case of error)
		}
	}()

	deadline := start.Add(spec.MaxRuntime)

	if err := conn.SetWriteDeadline(deadline); err != nil {
		logging.Logger.WithError(err).Warn("sender: conn.SetWriteDeadline failed")
		return
	}

	if err := message.SendTicks(conn, start, deadline); err != nil {
		logging.Logger.WithError(err).Warn("sender: ping.message.SendTicks failed")
		return
	}

	for m := range senderch {
		if err := conn.WriteJSON(m); err != nil {
			logging.Logger.WithError(err).Warn("sender: conn.WriteJSON failed")
			return
		}
		if err := message.SendTicks(conn, start, deadline); err != nil {
			logging.Logger.WithError(err).Warn("sender: ping.message.SendTicks failed")
			return
		}
	}

	closer.StartClosing(conn)
}

// Start starts the sender in a background goroutine. The sender will send
// to the client the measurement messages coming from senderch. Websocket ping
// frame will be sent right after the message. The sender does not signal errors,
// early cancellation in case of a network error is delegated to the receiver.
//
// Liveness guarantees:
// 1) sender keeps MaxRuntime as a timeout for conn operations;
// 2) sender drains the senderch, otherwise mux is deadlocked on send(TCPInfo).
func Start(conn *websocket.Conn, senderch <-chan model.Measurement, start time.Time) {
	go loop(conn, senderch, start)
}
