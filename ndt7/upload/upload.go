// Package upload implements the ndt7 upload
package upload

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/fdcache"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/results"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/tcpinfox"
)

// TODO(bassosimone): this code is the original upload implemented
// by evfirerob in https://github.com/m-lab/ndt-server/pull/62 that
// has been adapted to changes in the codebase since the original
// code has been written. There is some duplicated code between the
// download and the upload that we should factor.

const (
	// defaultTimeout is the default value of the I/O timeout.
	defaultTimeout = 7 * time.Second

	// maxDuration is the maximum duration of the upload in nanoseconds.
	maxDuration = 15 * time.Second
)

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
		err := errors.New("upload: cannot get file bound to websocket conn")
		return nil, err
	}
	return fp, nil
}

// gatherAndSaveTCPInfo gathers TCP info measurements from |sockfp| and stores
// them into the |measurement| object. On error, the measurement.TCPInfo will remain unchanged.
func gatherAndSaveTCPInfo(measurement *model.Measurement, sockfp *os.File) error {
	metrics, err := tcpinfox.GetTCPInfo(sockfp)
	if err == nil {
		measurement.TCPInfo = &metrics
	}
	return nil
}

// measuringLoop is the loop that runs the measurements in a goroutine.
// This function exits when
//     (1) a fatal error occurs or
//     (2) the maximum elapsed time for the upload test expires.
// The rest of the upload code is supposed to stop receiving when this function
// signals that we're done by closing the channel.
func measuringLoop(ctx context.Context, conn *websocket.Conn, resultfp *results.File, dst chan<- model.Measurement) {
	logging.Logger.Debug("upload: starting measurement loop")
	defer logging.Logger.Debug("upload: stopping measurement loop")
	defer close(dst)
	sockfp, err := getConnFile(conn)
	if err != nil {
		return // error printed already
	}
	defer warnonerror.Close(sockfp, "upload: ignoring sockfp.Close error")
	t0 := time.Now()
	resultfp.StartTest()
	defer resultfp.EndTest()
	ticker := time.NewTicker(spec.MinMeasurementInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			elapsed := now.Sub(t0)
			if elapsed > maxDuration {
				logging.Logger.Debug("upload: I have run for long enough")
				return
			}
			measurement := model.Measurement{
				Elapsed: elapsed.Seconds(),
			}
			err = gatherAndSaveTCPInfo(&measurement, sockfp)
			if err != nil {
				return // error printed already
			}
			dst <- measurement
		}
	}
}

// startMeasuring runs the measurement loop. This runs in a separate goroutine
// and emits Measurement events on the returned channel.
func startMeasuring(ctx context.Context, conn *websocket.Conn, resultfp *results.File) <-chan model.Measurement {
	dst := make(chan model.Measurement)
	go measuringLoop(ctx, conn, resultfp, dst)
	return dst
}

// Do implements the upload subtest. The ctx argument is the parent context
// for the subtest. The conn argument is the open WebSocket connection. The
// resultfp argument is the file where to save results. Both arguments are
// owned by the caller of this function.
func Do(ctx context.Context, conn *websocket.Conn, resultfp *results.File) {
	// Read limit is set to the smallest allowed payload size.
	conn.SetReadLimit(spec.MinMaxMessageSize)
	// Start measuring loop
	logging.Logger.Debug("upload: starting measuring goroutine")
	ctx, cancel := context.WithCancel(ctx)
	measurements := startMeasuring(ctx, conn, resultfp)
	// Make sure we clean up resources
	defer func() {
		logging.Logger.Debug("upload: closing the WebSocket connection")
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
	logging.Logger.Debug("upload: starting receiving data from the client")
	for {
		select {
		case m, ok := <-measurements:
			if !ok {
				return // the goroutine told us it's time to stop running
			}
			resultfp.SaveServerMeasurement(m)
		default:
			conn.SetReadDeadline(time.Now().Add(defaultTimeout))
			mt, message, err := conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					logging.Logger.WithError(err)
				}
				return
			}
			if mt == websocket.TextMessage {
				var m model.Measurement
				err := json.Unmarshal(message, &m)
				if err != nil {
					logging.Logger.Errorf(
						"upload: unable to unmarshal JSON message: %s", message,
					)
					return
				}
				// Save this message from the client.
				resultfp.SaveClientMeasurement(m)
			}
		}
	}
}
