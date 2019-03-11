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

// defaultTimeout is the default value of the I/O timeout.
const defaultTimeout = 7 * time.Second

// makePreparedMessage generates a prepared message that should be sent
// over the network for generating network load.
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

// Start is a pipeline stage that continuously sends binary messages to the
// client using conn and intermixes such messages with measurements coming
// from the measurement channel. This function fully drains the measurement
// channel, also in case it leaves early because of error. The default
// behaviour is to continue running as long as there are measurements coming
// in. That it, it's the goroutine writing on the measurements channel that
// decides when we should stop running.
//
// In addition, this pipeline stage will also posts nonblocking liveness
// updates on the output channel. The purpose of such updates is to inform
// the counter-flow messages reader that it should continue reading. This
// downstream pipeline stage is supposed to continue until it sees an error
// coming from us, or until the output channel is closed.
func Start(conn *websocket.Conn, measurements <-chan model.Measurement) <-chan error {
	out := make(chan error)
	go func() {
		defer close(out)
		defer func() {
			for range measurements {
				// make sure we drain the channel
			}
		}()
		logging.Logger.Debug("Generating random buffer")
		const bulkMessageSize = 1 << 13
		preparedMessage, err := makePreparedMessage(bulkMessageSize)
		if err != nil {
			out <- err
			return
		}
		logging.Logger.Debug("Start sending data to client")
		defer logging.Logger.Debug("Stop sending data to client")
		conn.SetReadLimit(spec.MinMaxMessageSize)
		for {
			select {
			case m, ok := <-measurements:
				if !ok {
					// This means that the previous step has terminated
					msg := websocket.FormatCloseMessage(
						websocket.CloseNormalClosure, "Done sending")
					out <- conn.WriteControl(websocket.CloseMessage, msg, time.Time{})
					return
				}
				conn.SetWriteDeadline(time.Now().Add(defaultTimeout))
				if err := conn.WriteJSON(m); err != nil {
					out <- err
					return
				}
			default:
				conn.SetWriteDeadline(time.Now().Add(defaultTimeout))
				if err := conn.WritePreparedMessage(preparedMessage); err != nil {
					out <- err
					return
				}
			}
			// We MUST NOT block on the output channel but we MUST make sure that
			// the downstream stage continues reading until we're done.
			select {
			case out <- nil:
			default:
			}
		}
	}()
	return out
}
