package upload

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/fdcache"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/server/results"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/tcpinfox"
)

const (
	// defaultTimeout is the default value of the I/O timeout.
	defaultTimeout = 7 * time.Second

	// defaultDuration is the default duration of a subtest in nanoseconds.
	defaultDuration = 10 * time.Second
)

// Handler handles an upload test on the server side.
type Handler struct {
	Upgrader websocket.Upgrader
	DataDir  string
}

// warnAndClose emits a warning |message| and then closes the HTTP connection
// using the |writer| http.ResponseWriter.
func warnAndClose(writer http.ResponseWriter, message string) {
	logging.Logger.Warn(message)
	writer.Header().Set("Connection", "Close")
	writer.WriteHeader(http.StatusBadRequest)
}

// getConnFile returns the connection to be used to gather low level stats.
// It returns a file to use to gather TCP_INFO stats on success, an error
// on failure.
func getConnFile(conn *websocket.Conn) (*os.File, error) {
	fp := fdcache.GetAndForgetFile(conn.UnderlyingConn())
	// Implementation note: in theory fp SHOULD always be non-nil because
	// now we always register the fp bound to a net.TCPConn. However, in
	// some weird cases it MAY happen that the cache pruning mechanism will
	// remove the fp BEFORE we can steal it. In case we cannot get a file
	// we just abort the test, as this should not happen (TM).
	if fp == nil {
		err := errors.New("cannot get file bound to websocket conn")
		return nil, err
	}
	return fp, nil
}

// gatherAndSaveTCPInfo gathers TCP info measurements from |sockfp| and stores
// them into the |measurement| object as well as into the |resultfp| file.
// It returns an error on failure and nil in case of success.
func gatherAndSaveTCPInfo(measurement *model.Measurement, sockfp *os.File, resultfp *results.File) error {
	metrics, err := tcpinfox.GetTCPInfo(sockfp)
	if err == nil {
		measurement.TCPInfo = &metrics
	}

	if err := resultfp.WriteMeasurement(*measurement, "server"); err != nil {
		logging.Logger.WithError(err).Warn("Cannot save measurement on disk")
		return err
	}
	return nil
}

// measuringLoop is the loop that runs the measurements in a goroutine.
// This function exits when
//     (1) a fatal error occurs or
//     (2) the maximum elapsed time for the upload test expires.
// The rest of the upload code is supposed to stop receiving when this function
// signals that we're done by closing the channel.
func measuringLoop(ctx context.Context, request *http.Request,
	conn *websocket.Conn, dataDir string, dst chan model.Measurement) {
	logging.Logger.Debug("Starting measurement loop")
	defer logging.Logger.Debug("Stopping measurement loop")
	defer close(dst)

	resultfp, err := results.OpenFor(request, conn, dataDir, "upload")
	if err != nil {
		return // error printed already
	}

	defer warnonerror.Close(resultfp, "Warning: ignored error")

	sockfp, err := getConnFile(conn)
	if err != nil {
		return // error printed already
	}

	defer warnonerror.Close(sockfp, "Warning: ignored error")
	t0 := time.Now()
	ticker := time.NewTicker(spec.MinMeasurementInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			elapsed := now.Sub(t0)
			if elapsed > defaultDuration {
				logging.Logger.Debug("Upload has run for long enough.")
				return
			}

			measurement := model.Measurement{
				Elapsed: elapsed.Seconds(),
			}

			err = gatherAndSaveTCPInfo(&measurement, sockfp, resultfp)
			if err != nil {
				return // error printed already
			}

			dst <- measurement
		}
	}
}

// startMeasuring runs the measurement loop. This runs in a separate goroutine
// and emits Measurement events on the returned channel.
func startMeasuring(ctx context.Context, request *http.Request, conn *websocket.Conn, dataDir string) chan model.Measurement {
	dst := make(chan model.Measurement)
	go measuringLoop(ctx, request, conn, dataDir, dst)
	return dst
}

// ServeHTTP handles the upload subtest.
func (ul Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// TODO(evfirerob): factor out this stuff as it's the same for both UL/DL
	logging.Logger.Debug("Upgrading to WebSockets")

	if request.Header.Get("Sec-WebSocket-Protocol") != spec.SecWebSocketProtocol {
		warnAndClose(writer, "Missing Sec-WebSocket-Protocol in request")
		return
	}
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)

	conn, err := ul.Upgrader.Upgrade(writer, request, headers)
	if err != nil {
		warnAndClose(writer, "Cannot UPGRADE to WebSocket")
		return
	}

	defer warnonerror.Close(conn, "Warning: ignoring error")

	// Read limit is set to the smallest allowed payload size.
	conn.SetReadLimit(spec.MinMaxMessageSize)

	// Start measuring loop
	logging.Logger.Debug("Starting measuring goroutine")
	ctx, cancel := context.WithCancel(request.Context())
	measurements := startMeasuring(ctx, request, conn, ul.DataDir)

	// Make sure we clean up resources
	defer func() {
		logging.Logger.Debug("Closing the WebSocket connection")
		conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(
			websocket.CloseNormalClosure, ""), time.Now().Add(defaultTimeout))

		// We could leave the context because the measuring goroutine thinks we're
		// done or because there has been a socket error. In the latter case, it is
		// important to synchronise with the goroutine and wait for it to exit.
		cancel()
		for range measurements {
			// NOTHING
		}
	}()

	logging.Logger.Debug("Starting receiving data from the client")
	for {
		select {
		case _, ok := <-measurements:
			if !ok {
				return // the goroutine told us it's time to stop running
			}
		default:
			conn.SetReadDeadline(time.Now().Add(defaultTimeout))
			mt, message, err := conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					logging.Logger.WithError(err)
				}
				break
			}

			if mt == websocket.TextMessage {
				var mdata model.Measurement
				err := json.Unmarshal(message, &mdata)
				if err != nil {
					logging.Logger.Errorf("Unable to unmarshal JSON message: %s", message)
					break
				}
			}
		}
	}
}
