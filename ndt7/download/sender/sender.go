// Package sender implements the download sender.
package sender

import (
	"math/rand"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/closer"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/ping/message"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

func makePreparedMessage(size int) (*websocket.PreparedMessage, error) {
	data := make([]byte, size)
	_, err := rand.Read(data)
	if err != nil {
		return nil, err
	}
	return websocket.NewPreparedMessage(websocket.BinaryMessage, data)
}

func loop(conn *websocket.Conn, src <-chan model.Measurement, dst chan<- model.Measurement, start time.Time) {
	logging.Logger.Debug("sender: start")
	defer logging.Logger.Debug("sender: stop")
	defer close(dst)
	defer func() {
		for range src {
			// make sure we drain the channel
		}
	}()
	logging.Logger.Debug("sender: generating random buffer")
	bulkMessageSize := 1 << 13
	preparedMessage, err := makePreparedMessage(bulkMessageSize)
	if err != nil {
		logging.Logger.WithError(err).Warn("sender: makePreparedMessage failed")
		return
	}
	deadline := start.Add(spec.MaxRuntime)
	err = conn.SetWriteDeadline(deadline) // Liveness!
	if err != nil {
		logging.Logger.WithError(err).Warn("sender: conn.SetWriteDeadline failed")
		return
	}
	// One RTT sample is taken before flooding the connection with data.
	// That sample is not affected by HOL, so it has additional value and is treated specially.
	if err := message.SendTicks(conn, start, deadline); err != nil {
		logging.Logger.WithError(err).Warn("sender: ping.message.SendTicks failed")
		return
	}
	var totalSent int64
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
			if err := message.SendTicks(conn, start, deadline); err != nil {
				logging.Logger.WithError(err).Warn("sender: ping.message.SendTicks failed")
				return
			}
		default:
			if err := conn.WritePreparedMessage(preparedMessage); err != nil {
				logging.Logger.WithError(err).Warn(
					"sender: conn.WritePreparedMessage failed",
				)
				return
			}
			// The following block of code implements the scaling of message size
			// as recommended in the spec's appendix. We're not accounting for the
			// size of JSON messages because that is small compared to the bulk
			// message size. The net effect is slightly slowing down the scaling,
			// but this is currently fine. We need to gather data from large
			// scale deployments of this algorithm anyway, so there's no point
			// in engaging in fine grained calibration before knowing.
			totalSent += int64(bulkMessageSize)
			if totalSent >= spec.MaxScaledMessageSize {
				continue // No further scaling is required
			}
			if int64(bulkMessageSize) > totalSent/spec.ScalingFraction {
				continue // message size still too big compared to sent data
			}
			bulkMessageSize *= 2
			preparedMessage, err = makePreparedMessage(bulkMessageSize)
			if err != nil {
				logging.Logger.WithError(err).Warn("sender: makePreparedMessage failed")
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
// the MaxRuntime of the subtest, provided that the consumer will
// continue reading from the returned channel. This is enforced by
// setting the write deadline to |start| + MaxRuntime.
func Start(conn *websocket.Conn, src <-chan model.Measurement, start time.Time) <-chan model.Measurement {
	dst := make(chan model.Measurement)
	go loop(conn, src, dst, start)
	return dst
}
