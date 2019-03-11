// Package receiver implements the counter-flow messages receiver.
package receiver

import (
	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
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
		for err := range in {
			if err != nil {
				out <- err
				return
			}
			_, _, err := conn.ReadMessage()
			if err != nil {
				out <- err
				return
			}
		}
	}()
	return out
}
