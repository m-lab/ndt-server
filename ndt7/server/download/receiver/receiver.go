// Package receiver implements the counter-flow messages receiver.
package receiver

import (
	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

// Start starts a goroutine that reads counter-flow messages sent by advanced
// clients and continues running until the liveness information coming
// from the in channel indicates an error, or the channel is closed.
// At that point, if there is no error, Run will attempt to cleanly close
// the websocket connection. It returns to the caller whether the whole
// downloader pipeline succeeded or not, by posting the final return
// value onto the returned channel. In case of success, this function
// will just close the channel.
func Start(conn *websocket.Conn, in <-chan error) <-chan error {
	out := make(chan error)
	go func() {
		defer close(out)
		defer func() {
			for range in {
				// drain
			}
		}()
		defer logging.Logger.Debug("Stop reading counter-flow messages")
		logging.Logger.Debug("Start reading counter-flow messages")
		conn.SetReadLimit(spec.MinMaxMessageSize)
		for {
			// We may want to continue reading the connection even after the
			// upstream channel is closed, because of upcoming counter-flow
			// messages that we need to finish reading.
			select {
			case err, ok := <-in:
				if ok && err != nil {
					out <- err
					return
				}
			default:
			}
			_, _, err := conn.ReadMessage()
			if err != nil {
				// A normal closure is what we'd like to see here. The receiver should
				// issue a normal closure when it has finished uploading all the
				// pending counter-flow measurements. If the receiver is a simple
				// receiver that doesn't upload counter-flow measurements, we'll
				// be blocked in conn.ReadMessage until the next stage of the pipeline
				// will timeout and close the connection.
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					out <- err
				}
				return
			}
		}
	}()
	return out
}
