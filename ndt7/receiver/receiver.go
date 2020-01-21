// Package receiver implements the messages receiver. It can be used
// both by the download and the upload subtests.
package receiver

import (
	"context"
	"encoding/json"
	"time"
	"math"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/ping"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

type receiverKind int

const (
	downloadReceiver = receiverKind(iota)
	uploadReceiver
	pingReceiver
)

const (
	MaxDuration = math.MaxInt64 * time.Nanosecond
)

func loop(
	ctx context.Context, conn *websocket.Conn, kind receiverKind,
	dst chan<- model.Measurement, start time.Time, pongch chan<- model.WSPingInfo,
) {
	logging.Logger.Debug("receiver: start")
	defer logging.Logger.Debug("receiver: stop")
	defer close(dst)
	defer close(pongch)
	conn.SetReadLimit(spec.MaxMessageSize)
	receiverctx, cancel := context.WithTimeout(ctx, spec.MaxRuntime)
	defer cancel()
	err := conn.SetReadDeadline(start.Add(spec.MaxRuntime)) // Liveness!
	if err != nil {
		logging.Logger.WithError(err).Warn("receiver: conn.SetReadDeadline failed")
		return
	}
	minRTT := MaxDuration
	conn.SetPongHandler(func(s string) error {
		elapsed, rtt, err := ping.ParseTicks(s, start)
		if err == nil {
			logging.Logger.Debugf("receiver: ApplicationLevel RTT: %d ms", int64(rtt / time.Millisecond))
			if rtt < minRTT {
				minRTT = rtt
			}

			wsinfo := model.WSPingInfo{
				ElapsedTime: int64(elapsed / time.Microsecond),
				LastRTT: int64(rtt / time.Microsecond),
				MinRTT: int64(minRTT / time.Microsecond),
			}
			pongch <- wsinfo // Liveness: buffered (sender)
		}
		return err
	})
	for receiverctx.Err() == nil { // Liveness!
		mtype, mdata, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				return
			}
			logging.Logger.WithError(err).Warn("receiver: conn.ReadMessage failed")
			return
		}
		if mtype != websocket.TextMessage {
			switch kind {
			case uploadReceiver:
				continue // No further processing required
			default: // downloadReceiver and pingReceiver
				logging.Logger.Warn("receiver: got non-Text message")
				return // Unexpected message type
			}
		}
		var measurement model.Measurement
		err = json.Unmarshal(mdata, &measurement)
		if err != nil {
			logging.Logger.WithError(err).Warn("receiver: json.Unmarshal failed")
			return
		}
		dst <- measurement // Liveness: this is blocking
	}
}

func startReceiver(ctx context.Context, conn *websocket.Conn, kind receiverKind, start time.Time) (<-chan model.Measurement, <-chan model.WSPingInfo) {
	// |dst| is going to the log file
	dst := make(chan model.Measurement)
	// |pongch| goes to the client, it's buffered to avoid blocking on `download.sender.loop`
	// while `conn.WritePreparedMessage()` is active.
	// TODO(darkk): is it possible to reduce buffer size or to avoiding blocking in some other way? May avoiding L7 pings at /download altogether be the way?
	pongch := make(chan model.WSPingInfo, 1 + spec.MaxRuntime / spec.MinPoissonSamplingInterval)
	go loop(ctx, conn, kind, dst, start, pongch)
	return dst, pongch
}

// StartDownloadReceiver starts the receiver in a background goroutine and
// returns the messages received from the client in the returned channel.
//
// This receiver will not tolerate receiving binary messages. It will
// terminate early if such a message is received.
//
// Liveness guarantee: the goroutine will always terminate after a
// MaxRuntime timeout, provided that the consumer will keep reading
// from the returned channel.
func StartDownloadReceiver(ctx context.Context, conn *websocket.Conn, start time.Time, msmch <-chan model.Measurement) (<-chan model.Measurement, <-chan model.WSPingInfo) {
	return startReceiver(ctx, conn, downloadReceiver, start)
}

// StartUploadReceiver is like StartDownloadReceiver except that it
// tolerates incoming binary messages, which are sent to cause
// network load, and therefore must not be rejected.
func StartUploadReceiver(ctx context.Context, conn *websocket.Conn, start time.Time) (<-chan model.Measurement, <-chan model.WSPingInfo) {
	return startReceiver(ctx, conn, uploadReceiver, start)
}

// StartPingReceiver is exactly like StartDownloadReceiver currently.
func StartPingReceiver(ctx context.Context, conn *websocket.Conn, start time.Time) (<-chan model.Measurement, <-chan model.WSPingInfo) {
	return startReceiver(ctx, conn, pingReceiver, start)
}
