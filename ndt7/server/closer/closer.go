// Package closer implements the final stage of the download and upload
// pipelines where we close the connection.
package closer

import (
	"errors"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
)

// maxAcceptableRuntime is the time after which a pipeline is terminated.
const maxAcceptableRuntime = 15 * time.Second

// Run waits for an error to be emitted on the input channel, for the input
// channel to be closed, meaning no error occurred, or for a global reasonable
// timeout to expire. In all cases except the no-error case, an error will be
// returned to the caller. In the no-error case, nil will be returned. Note
// that in all cases, the connection will be closed.
func Run(conn *websocket.Conn, in <-chan error) error {
	defer func() {
		for range in {
			// Drain
		}
	}()
	defer logging.Logger.Debug("Stop waiting for pipeline to complete")
	logging.Logger.Debug("Start waiting for pipeline to complete")
	var err error
	timer := time.NewTimer(maxAcceptableRuntime)
	select {
	case err = <-in:
	case <-timer.C:
		err = errors.New("Maximum runtime exceeded")
	}
	if err != nil {
		closeErr := conn.Close()
		if closeErr != nil {
			logging.Logger.WithError(closeErr).Debug(
				"Ignoring conn.Close error because of another error")
		}
		return err
	}
	return conn.Close()
}
