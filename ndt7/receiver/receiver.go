// Package receiver implements the messages receiver. It can be used
// both by the download and the upload subtests.
package receiver

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	ndt7metrics "github.com/m-lab/ndt-server/ndt7/metrics"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/ping"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

type receiverKind int

const (
	downloadReceiver = receiverKind(iota)
	uploadReceiver
)

func start(
	ctx context.Context, conn *websocket.Conn, kind receiverKind,
	data *model.ArchivalData, MaxMsgSize int64,
) {
	logging.Logger.Debug("receiver: start")
	proto := ndt7metrics.ConnLabel(conn)
	defer logging.Logger.Debug("receiver: stop")
	conn.SetReadLimit(MaxMsgSize)
	receiverctx, cancel := context.WithTimeout(ctx, spec.MaxRuntime)
	defer cancel()
	err := conn.SetReadDeadline(time.Now().Add(spec.MaxRuntime)) // Liveness!
	if err != nil {
		logging.Logger.WithError(err).Warn("receiver: conn.SetReadDeadline failed")
		ndt7metrics.ClientReceiverErrors.WithLabelValues(
			proto, fmt.Sprint(kind), "set-read-deadline").Inc()
		return
	}
	conn.SetPongHandler(func(s string) error {
		rtt, err := ping.ParseTicks(s)
		if err == nil {
			rtt /= int64(time.Millisecond)
			logging.Logger.Debugf("receiver: ApplicationLevel RTT: %d ms", rtt)
		} else {
			ndt7metrics.ClientReceiverErrors.WithLabelValues(
				proto, fmt.Sprint(kind), "ping-parse-ticks").Inc()
		}
		return err
	})
	for receiverctx.Err() == nil { // Liveness!
		// By getting a Reader here we avoid allocating memory for the message
		// when the message type is not websocket.TextMessage.
		mtype, r, err := conn.NextReader()
		if err != nil {
			ndt7metrics.ClientReceiverErrors.WithLabelValues(
				proto, fmt.Sprint(kind), "read-message-type").Inc()
			return
		}
		if mtype != websocket.TextMessage {
			switch kind {
			case downloadReceiver:
				logging.Logger.Warn("receiver: got non-Text message")
				ndt7metrics.ClientReceiverErrors.WithLabelValues(
					proto, fmt.Sprint(kind), "wrong-message-type").Inc()
				return // Unexpected message type
			default:
				// NOTE: this is the bulk upload path. In this case, the mdata is not used.
				continue // No further processing required
			}
		}
		// This is a TextMessage, so we must read it.
		mdata, err := ioutil.ReadAll(r)
		if err != nil {
			ndt7metrics.ClientReceiverErrors.WithLabelValues(
				proto, fmt.Sprint(kind), "read-message").Inc()
			return
		}
		var measurement model.Measurement
		err = json.Unmarshal(mdata, &measurement)
		if err != nil {
			logging.Logger.WithError(err).Warn("receiver: json.Unmarshal failed")
			ndt7metrics.ClientReceiverErrors.WithLabelValues(
				proto, fmt.Sprint(kind), "unmarshal-client-message").Inc()
			return
		}
		data.ClientMeasurements = append(data.ClientMeasurements, measurement)
	}
	ndt7metrics.ClientReceiverErrors.WithLabelValues(
		proto, fmt.Sprint(kind), "receiver-context-expired").Inc()
}

// StartDownloadReceiverAsync starts the receiver in a background goroutine and
// saves messages received from the client in the given archival data. The
// returned context may be used to detect when the receiver has completed.
//
// This receiver will not tolerate receiving binary messages. It will terminate
// early if such a message is received.
//
// Liveness guarantee: the goroutine will always terminate after a MaxRuntime
// timeout.
func StartDownloadReceiverAsync(ctx context.Context, conn *websocket.Conn, data *model.ArchivalData) context.Context {
	ctx2, cancel2 := context.WithCancel(ctx)
	go func() {
		start(ctx2, conn, downloadReceiver, data, spec.MaxMessageSize)
		cancel2()
	}()
	return ctx2
}

// StartUploadReceiverAsync is like StartDownloadReceiverAsync except that it
// tolerates incoming binary messages, sent by "upload" measurement clients to
// create network load, and therefore must be allowed.
func StartUploadReceiverAsync(ctx context.Context, conn *websocket.Conn, data *model.ArchivalData, MaxMsgSize int64) context.Context {
	ctx2, cancel2 := context.WithCancel(ctx)
	go func() {
		start(ctx2, conn, uploadReceiver, data, MaxMsgSize)
		cancel2()
	}()
	return ctx2
}
