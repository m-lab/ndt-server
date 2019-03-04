// Package download implements the ndt7/server downloader.
package download

import (
	"context"
	"errors"
	"math/rand"
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

// defaultTimeout is the default value of the I/O timeout.
const defaultTimeout = 7 * time.Second

// defaultDuration is the default duration of a subtest in nanoseconds.
const defaultDuration = 10 * time.Second

// Handler handles a download subtest from the server side.
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

// startMeasuring runs the measurement loop. This runs in a separate goroutine
// and emits Measurement events on the returned channel.
func startMeasuring(ctx context.Context, request *http.Request, conn *websocket.Conn, dataDir string) chan model.Measurement {
	dst := make(chan model.Measurement)
	go measuringLoop(ctx, request, conn, dataDir, dst)
	return dst
}

// Handle handles the download subtest.
func (dl Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	logging.Logger.Debug("Upgrading to WebSockets")
	if request.Header.Get("Sec-WebSocket-Protocol") != spec.SecWebSocketProtocol {
		warnAndClose(writer, "Missing Sec-WebSocket-Protocol in request")
		return
	}
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	conn, err := dl.Upgrader.Upgrade(writer, request, headers)
	if err != nil {
		warnAndClose(writer, "Cannnot UPGRADE to WebSocket")
		return
	}
	// TODO(bassosimone): an error before this point means that the *os.File
	// will stay in cache until the cache pruning mechanism is triggered. This
	// should be a small amount of seconds. If Golang does not call shutdown(2)
	// and close(2), we'll end up keeping sockets that caused an error in the
	// code above (e.g. because the handshake was not okay) alive for the time
	// in which the corresponding *os.File is kept in cache.
	defer conn.Close()
	logging.Logger.Debug("Generating random buffer")
	const bulkMessageSize = 1 << 13
	preparedMessage, err := makePreparedMessage(bulkMessageSize)
	if err != nil {
		logging.Logger.WithError(err).Warn("Cannot prepare message")
		return
	}
	logging.Logger.Debug("Start measurement goroutine")
	ctx, cancel := context.WithCancel(request.Context())
	measurements := startMeasuring(ctx, request, conn, dl.DataDir)
	logging.Logger.Debug("Start sending data to client")
	conn.SetReadLimit(spec.MinMaxMessageSize)
	// make sure we cleanup resources when we leave
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
	for {
		select {
		case m, ok := <-measurements:
			if !ok {
				return // the goroutine told us it's time to stop running
			}
			conn.SetWriteDeadline(time.Now().Add(defaultTimeout))
			if err := conn.WriteJSON(m); err != nil {
				logging.Logger.WithError(err).Warn("Cannot send measurement message")
				return
			}
		default:
			conn.SetWriteDeadline(time.Now().Add(defaultTimeout))
			if err := conn.WritePreparedMessage(preparedMessage); err != nil {
				logging.Logger.WithError(err).Warn("Cannot send prepared message")
				return
			}
		}
	}
}
