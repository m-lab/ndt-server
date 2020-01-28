// Package mux implements the ping subtest receiver.
package receiver

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/ping/message"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

type Measurement struct {
	Measurement    model.Measurement
	IsServerOrigin bool
}

const (
	maxDuration = math.MaxInt64 * time.Nanosecond
)

func loop(ctx context.Context, conn *websocket.Conn, receiverch chan<- Measurement, start time.Time) {
	logging.Logger.Debug("receiver: start")
	defer logging.Logger.Debug("receiver: stop")
	defer close(receiverch)

	conn.SetReadLimit(spec.MaxMessageSize)

	deadline, ok := ctx.Deadline()
	if !ok {
		panic("You passed me a context.Context without deadline")
	}

	if err := conn.SetReadDeadline(deadline); err != nil {
		logging.Logger.WithError(err).Warn("receiver: conn.SetReadDeadline failed")
		return
	}

	minRTT := maxDuration

	conn.SetPongHandler(func(s string) error {
		elapsed, rtt, err := message.ParseTicks(s, start)
		if err == nil {
			if rtt < minRTT {
				minRTT = rtt
			}
			m := Measurement{
				Measurement: model.Measurement{
					WSPingInfo: &model.WSPingInfo{
						ElapsedTime: int64(elapsed / time.Microsecond),
						LastRTT:     int64(rtt / time.Microsecond),
						MinRTT:      int64(minRTT / time.Microsecond),
					},
				},
				IsServerOrigin: true,
			}
			receiverch <- m
		}
		return err
	})

	for ctx.Err() == nil {
		mtype, mdata, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				return
			}
			logging.Logger.WithError(err).Warn("receiver: conn.ReadMessage failed")
			return
		}
		if mtype != websocket.TextMessage {
			logging.Logger.Warn("receiver: got non-Text message")
			return // Unexpected message type
		}
		m := Measurement{
			IsServerOrigin: false,
		}
		err = json.Unmarshal(mdata, &m.Measurement)
		if err != nil {
			logging.Logger.WithError(err).Warn("receiver: json.Unmarshal failed")
			return
		}
		receiverch <- m
	}
}

// Start starts the receiver in a background goroutine. The receiver processes pong frames
// and the measurement messages coming from conn.
//
// Liveness guarantees:
// 1) receiver uses ctx as the deadline for all conn operations and the goroutine itself,
// 2) receiver closes output channels when it's done.
func Start(ctx context.Context, conn *websocket.Conn, start time.Time) <-chan Measurement {
	receiverch := make(chan Measurement)
	go loop(ctx, conn, receiverch, start)
	return receiverch
}
