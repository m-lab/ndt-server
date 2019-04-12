// Package sender implements the download sender.
package sender

import (
	"math/rand"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

func makePreparedMessage(size int) (*websocket.PreparedMessage, error) {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	data := make([]byte, size)
	// This is not the fastest algorithm to generate a random string, yet it
	// is most likely good enough for our purposes. See [1] for a comprehensive
	// discussion regarding how to generate a random string in Golang.
	//
	// .. [1] https://stackoverflow.com/a/31832326/4354461
	for i := range data {
		data[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return websocket.NewPreparedMessage(websocket.BinaryMessage, data)
}

func startclosing(conn *websocket.Conn) {
	msg := websocket.FormatCloseMessage(
		websocket.CloseNormalClosure, "Done sending")
	d := time.Now().Add(spec.DefaultRuntime) // Liveness!
	err := conn.WriteControl(websocket.CloseMessage, msg, d)
	if err != nil {
		logging.Logger.WithError(err).Warn("sender: conn.WriteControl failed")
		return
	}
	logging.Logger.Debug("sender: sending Close message")
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
	logging.Logger.Debug("sender: generating random buffer")
	const bulkMessageSize = 1 << 13
	preparedMessage, err := makePreparedMessage(bulkMessageSize)
	if err != nil {
		logging.Logger.WithError(err).Warn("sender: makePreparedMessage failed")
		return
	}
	for {
		select {
		case m, ok := <-src:
			if !ok { // This means that the previous step has terminated
				startclosing(conn)
				return
			}
			conn.SetWriteDeadline(time.Now().Add(spec.DefaultRuntime)) // Liveness!
			if err := conn.WriteJSON(m); err != nil {
				logging.Logger.WithError(err).Warn("sender: conn.WriteJSON failed")
				return
			}
			dst <- m // Liveness: this is blocking
		default:
			conn.SetWriteDeadline(time.Now().Add(spec.DefaultRuntime)) // Liveness!
			if err := conn.WritePreparedMessage(preparedMessage); err != nil {
				logging.Logger.WithError(err).Warn(
					"sender: conn.WritePreparedMessage failed",
				)
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
