package ndt7

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-cloud/bbr"
	"github.com/m-lab/ndt-cloud/fdcache"
	"github.com/m-lab/ndt-cloud/tcpinfox"
)

// defaultTimeout is the default value of the I/O timeout.
const defaultTimeout = 7 * time.Second

// defaultDuration is the default duration of a subtest in nanoseconds.
const defaultDuration = 10 * time.Second

// DownloadHandler handles a download subtest from the server side.
type DownloadHandler struct {
	Upgrader websocket.Upgrader
}

// warnAndClose emits a warning |message| and then closes the HTTP connection
// using the |writer| http.ResponseWriter.
func warnAndClose(writer http.ResponseWriter, message string) {
	ErrorLogger.Warn(message)
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

// openResultsFileAndWriteMetadata opens the results file and writes into it
// the results metadata based on the query string. Returns the results file
// on success. Returns an error in case of failure. The request arg is
// used to gather the query string. The conn arg is used to retrieve
// the local and remote endpoint addresses.
func openResultsFileAndWriteMetadata(request *http.Request, conn *websocket.Conn) (*resultsfile, error) {
	ErrorLogger.Debug("Processing query string")
	meta := make(metadata)
	initMetadata(&meta, conn.LocalAddr().String(),
		conn.RemoteAddr().String(), request.URL.Query(), "download")
	resultfp, err := newResultsfile()
	if err != nil {
		ErrorLogger.WithError(err).Warn("Cannot open results file")
		return nil, err
	}
	ErrorLogger.Debug("Writing metadata on results file")
	if err := resultfp.WriteMetadata(meta); err != nil {
		ErrorLogger.WithError(err).Warn("Cannot write metadata to results file")
		resultfp.Close()
		return nil, err
	}
	return resultfp, nil
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
		return nil, errors.New("cannot get file bound to websocket conn")
	}
	err := bbr.Enable(fp)
	if err != nil {
		ErrorLogger.WithError(err).Warn("Cannot enable BBR")
		// FALLTHROUGH
	}
	return fp, nil
}

// gatherAndSaveTCPInfoAndBBRInfo gathers TCP info and BBR measurements from
// |fp| and stores them into the |measurement| object as well as into the
// |resultfp| file. Returns an error on failure and nil in case of success.
func gatherAndSaveTCPInfoAndBBRInfo(measurement *Measurement, sockfp *os.File, resultfp *resultsfile) error {
	bw, rtt, err := bbr.GetMaxBandwidthAndMinRTT(sockfp)
	if err == nil {
		measurement.BBRInfo = &BBRInfo{
			MaxBandwidth: bw,
			MinRTT:       rtt,
		}
	}
	metrics, err := tcpinfox.GetTCPInfo(sockfp)
	if err == nil {
		measurement.TCPInfo = &metrics
	}
	if err := resultfp.WriteMeasurement(*measurement, "server"); err != nil {
		ErrorLogger.WithError(err).Warn("Cannot save measurement on disk")
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
func measuringLoop(ctx context.Context, request *http.Request, conn *websocket.Conn, dst chan Measurement) {
	defer close(dst)
	resultfp, err := openResultsFileAndWriteMetadata(request, conn)
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
	ticker := time.NewTicker(MinMeasurementInterval)
	ErrorLogger.Debug("Starting measurement loop")
	defer ErrorLogger.Debug("Stopping measurement loop")  // say goodbye properly
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			elapsed := now.Sub(t0)
			if elapsed > defaultDuration {
				ErrorLogger.Debug("Download run for enough time")
				return
			}
			measurement := Measurement{
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
func startMeasuring(ctx context.Context, request *http.Request, conn *websocket.Conn) chan Measurement {
	dst := make(chan Measurement)
	go measuringLoop(ctx, request, conn, dst)
	return dst
}

// Handle handles the download subtest.
func (dl DownloadHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	ErrorLogger.Debug("Upgrading to WebSockets")
	if request.Header.Get("Sec-WebSocket-Protocol") != SecWebSocketProtocol {
		warnAndClose(writer, "Missing Sec-WebSocket-Protocol in request")
		return
	}
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", SecWebSocketProtocol)
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
	ErrorLogger.Debug("Generating random buffer")
	const randomDataSize = 1 << 13
	preparedMessage, err := makePreparedMessage(randomDataSize)
	if err != nil {
		ErrorLogger.WithError(err).Warn("Cannot prepare message")
		return
	}
	ErrorLogger.Debug("Start measurement goroutine")
	ctx, cancel := context.WithCancel(context.Background())
	measurements := startMeasuring(ctx, request, conn)
	ErrorLogger.Debug("Start sending data to client")
	conn.SetReadLimit(MinMaxMessageSize)
	for {
		select {
		case m, ok := <-measurements:
			if !ok {
				goto out // the goroutine told us it's time to stop running
			}
			conn.SetWriteDeadline(time.Now().Add(defaultTimeout))
			if err := conn.WriteJSON(m); err != nil {
				ErrorLogger.WithError(err).Warn("Cannot send measurement message")
				goto out
			}
		default:
			conn.SetWriteDeadline(time.Now().Add(defaultTimeout))
			if err := conn.WritePreparedMessage(preparedMessage); err != nil {
				ErrorLogger.WithError(err).Warn("Cannot send prepared message")
				goto out
			}
		}
	}
out:
	ErrorLogger.Debug("Closing the WebSocket connection")
	conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(
		websocket.CloseNormalClosure, ""), time.Now().Add(defaultTimeout))
	// If we jumped here because of `goto out`, make sure we wait for the
	// goroutine to finish emitting its events. Use the context to tell it
	// that it should stop possibly earlier than expected.
	cancel()
	for _ = range measurements {
		// NOTHING
	}
}
