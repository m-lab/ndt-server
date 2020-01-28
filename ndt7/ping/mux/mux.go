// Package mux implements the ping subtest channel multiplexor.
package mux

import (
	"context"

	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/ping/receiver"
)

func upsert(self *model.Measurement, m model.Measurement) {
	if m.AppInfo != nil {
		self.AppInfo = m.AppInfo
	}
	if m.ConnectionInfo != nil {
		self.ConnectionInfo = m.ConnectionInfo
	}
	if m.BBRInfo != nil {
		self.BBRInfo = m.BBRInfo
	}
	if m.TCPInfo != nil {
		self.TCPInfo = m.TCPInfo
	}
	if m.WSPingInfo != nil {
		self.WSPingInfo = m.WSPingInfo
	}
}

func loop(
	measurerch <-chan model.Measurement, receiverch <-chan receiver.Measurement,
	senderch, serverlog, clientlog chan<- model.Measurement,
	cancel context.CancelFunc,
) {
	logging.Logger.Debug("mux: start")
	defer logging.Logger.Debug("mux: stop")

	state := model.Measurement{}
	for measurerch != nil || receiverch != nil {
		select {
		case m, ok := <-measurerch:
			if ok {
				serverlog <- m
				upsert(&state, m)
				if state.WSPingInfo != nil {
					// Pong arrived, there is no in-flight ping. Perfect time for another sample.
					senderch <- state
					state = model.Measurement{}
				}
			} else {
				logging.Logger.Debug("mux: measurerch closed")
				measurerch = nil
				// Propagate EOF to close websocket when measurer stops ticking.
				close(senderch)
			}
		case m, ok := <-receiverch:
			if ok {
				if m.IsServerOrigin { // likely, pong frame
					serverlog <- m.Measurement
					// The sample is forwarded to the client with the next TCPInfo frame.
					upsert(&state, m.Measurement)
				} else { // json from the client
					clientlog <- m.Measurement
				}
			} else {
				logging.Logger.Debug("mux: receiverch closed")
				receiverch = nil
				// Stop measurer's ticker. Receiver has already finished its duties.
				cancel()
			}
		}
	}
	close(serverlog)
	close(clientlog)
}

// MuxOutput is a return value of mux.Start to avoid confusion in argument ordering.
type MuxOutput struct {
	SenderC   <-chan model.Measurement
	ServerLog <-chan model.Measurement
	ClientLog <-chan model.Measurement
}

// Start starts the channel multiplexor in a background goroutine.
//
// Liveness guarantees:
// 1) mux drains measurerch, otherwise measurer is deadlocked on send();
// 2) mux drains receiverch, otherwise receiver MAY become deadlocked on send(clientMessage);
// 3) mux closes output channels when it's done;
// 4) EOF from receiverch is a signal to call |cancel| to terminate early.
func Start(measurerch <-chan model.Measurement, receiverch <-chan receiver.Measurement, cancel context.CancelFunc) MuxOutput {
	senderch := make(chan model.Measurement)
	serverlog := make(chan model.Measurement)
	clientlog := make(chan model.Measurement)
	go loop(measurerch, receiverch, senderch, serverlog, clientlog, cancel)
	return MuxOutput{
		// ServerLog is not named "ServerC" to avoid visual confusion with "SenderC".
		SenderC:   senderch,
		ServerLog: serverlog,
		ClientLog: clientlog,
	}
}
