// Package sender implements the download sender.
package sender

import (
	"math"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/closer"
	"github.com/m-lab/ndt-server/ndt7/download/sender/binary"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

func castToPositiveInt32(orig int64) int {
	if orig < 0 {
		return 0
	}
	if orig >= math.MaxInt32 {
		return math.MaxInt32
	}
	return int(orig)
}

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
			if meas.BBRInfo != nil {
				// Convert the bandwidth to bytes per second and then obtain the
				// number of bytes per second we want to send for each measurement
				// interval. This is the desired size of the next send. The real
				// size will be clipped into the proper interval. Also, note that
				// changing the prepared message size does not cause a reallocation
				// until we're calling bm.Send().
				n := castToPositiveInt32(meas.BBRInfo.MaxBandwidth)
				n /= 8
				n /= spec.MeasurementsPerSecond
				bm.PossiblyIncreaseSizeTo(n)
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
