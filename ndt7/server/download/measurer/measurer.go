// Package measurer contains the downloader measurer
package measurer

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/bbr"
	"github.com/m-lab/ndt-server/fdcache"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/server/results"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/tcpinfox"
)

// defaultDuration is the default duration of a subtest in nanoseconds.
const defaultDuration = 10 * time.Second

// getConnFileAndPossiblyEnableBBR returns the connection to be used to
// gather low level stats and possibly enables BBR. It returns a file to
// use to gather BBR/TCP_INFO stats on success, an error on failure.
func getConnFileAndPossiblyEnableBBR(conn *websocket.Conn) (*os.File, error) {
	fp := fdcache.GetAndForgetFile(conn.UnderlyingConn())
	// Implementation note: in theory fp SHOULD always be non-nil because
	// now we always register the fp bound to a net.TCPConn. However, in
	// some weird cases it MAY happen that the cache pruning mechanism will
	// remove the fp BEFORE we can steal it. In case we cannot get a file
	// we just abort the test, as this should not happen (TM).
	if fp == nil {
		err := errors.New("cannot get file bound to websocket conn")
		logging.Logger.WithError(err).Warn("Cannot enable BBR")
		return nil, err
	}
	err := bbr.Enable(fp)
	if err != nil {
		logging.Logger.WithError(err).Warn("Cannot enable BBR")
		// FALLTHROUGH
	}
	return fp, nil
}

// gatherAndSaveTCPInfoAndBBRInfo gathers TCP info and BBR measurements from
// |fp| and stores them into the |measurement| object as well as into the
// |resultfp| file. Returns an error on failure and nil in case of success.
func gatherAndSaveTCPInfoAndBBRInfo(measurement *model.Measurement, sockfp *os.File, resultfp *results.File) error {
	bbrinfo, err := bbr.GetMaxBandwidthAndMinRTT(sockfp)
	if err == nil {
		measurement.BBRInfo = &bbrinfo
	}
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

// This is the loop that runs the measurements in a goroutine. This function
// exits when (1) a fatal error occurs or (2) the maximum elapsed time for the
// download test expires. Because this function has access to BBR stats (if BBR
// is available), then that's the right place to stop the test early. The rest
// of the download code is supposed to stop downloading when this function will
// signal that we're done by closing the channel. This function will not tell
// the test of the downloader whether an error occurred because closing it will
// log any error and closing the channel provides already enough bits of info
// to synchronize this part of the downloader with the rest. The context param
// will be used by the outer loop to tell us when we need to stop early.
func measuringLoop(ctx context.Context, request *http.Request, conn *websocket.Conn, dataDir string, dst chan model.Measurement) {
	logging.Logger.Debug("Starting measurement loop")
	defer logging.Logger.Debug("Stopping measurement loop") // say goodbye properly
	defer close(dst)
	resultfp, err := results.OpenFor(request, conn, dataDir, "download")
	if err != nil {
		return // error already printed
	}
	defer resultfp.Close()
	sockfp, err := getConnFileAndPossiblyEnableBBR(conn)
	if err != nil {
		return // error already printed
	}
	defer sockfp.Close()
	t0 := time.Now()
	ticker := time.NewTicker(spec.MinMeasurementInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			elapsed := now.Sub(t0)
			if elapsed > defaultDuration {
				logging.Logger.Debug("Download run for enough time")
				return
			}
			measurement := model.Measurement{
				Elapsed: elapsed.Seconds(),
			}
			err = gatherAndSaveTCPInfoAndBBRInfo(&measurement, sockfp, resultfp)
			if err != nil {
				return // error already printed
			}
			dst <- measurement
		}
	}
}

// Start starts the measurement loop. This runs in a separate goroutine
// and emits Measurement events on the returned channel.
func Start(ctx context.Context, request *http.Request, conn *websocket.Conn, dataDir string) chan model.Measurement {
	dst := make(chan model.Measurement)
	go measuringLoop(ctx, request, conn, dataDir, dst)
	return dst
}
