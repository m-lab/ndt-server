// Package receiver implements the messages receiver. It can be used
// both by the download and the upload subtests.
package receiver

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

type receiverKind int

const (
	downloadReceiver = receiverKind(iota)
	uploadReceiver
)

func loop(
	ctx context.Context, conn *websocket.Conn, kind receiverKind,
	dst chan<- model.Measurement,
) {
	logging.Logger.Debug("receiver: start")
	defer logging.Logger.Debug("receiver: stop")
	defer close(dst)
	conn.SetReadLimit(spec.MaxMessageSize)
	receiverctx, cancel := context.WithTimeout(ctx, spec.MaxRuntime)
	defer cancel()
	err := conn.SetReadDeadline(time.Now().Add(spec.MaxRuntime)) // Liveness!
	if err != nil {
		logging.Logger.WithError(err).Warn("receiver: conn.SetReadDeadline failed")
		return
	}
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
			switch (kind) {
			case downloadReceiver:
				logging.Logger.Warn("receiver: got non-Text message")
				return // Unexpected message type
			default:
				continue // No further processing required
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

func start(ctx context.Context, conn *websocket.Conn, kind receiverKind) <-chan model.Measurement {
	dst := make(chan model.Measurement)
	go loop(ctx, conn, kind, dst)
	return dst
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
func StartDownloadReceiver(ctx context.Context, conn *websocket.Conn) <-chan model.Measurement {
	return start(ctx, conn, downloadReceiver)
}

// StartUploadReceiver is like StartDownloadReceiver except that it
// tolerates incoming binary messages, which are sent to cause
// network load, and therefore must not be rejected.
func StartUploadReceiver(ctx context.Context, conn *websocket.Conn) <-chan model.Measurement {
	return start(ctx, conn, uploadReceiver)
}
