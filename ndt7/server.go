package ndt7

import (
	"encoding/json"
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

// stableAccordingToBBR returns true when we can stop the current download
// test based on |prev|, the previous BBR max-bandwidth sample, |cur| the
// current BBR max-bandwidth sample, |rtt|, the BBR measured min-RTT (in
// millisecond), and |elapsed|, the elapsed time since the beginning
// of the test (expressed as a time.Duration). The max-bandwidth is measured
// in bits per second.
//
// This algorithm runs every 0.25 seconds. Empirically, we know that
// BBR requires multiple RTTs to converge. Here we use 10 RTTs as a reasonable
// upper bound. Before 10 RTTs have elapsed, we do not check whether the
// max-bandwidth has stopped growing. After 10 RTTs have elapsed, we call
// the connection stable when the max-bandwidth measured by BBR does not
// grow of more than 25% between two 0.25 second periods.
//
// We use the same percentage used by the BBR paper to characterize the
// max-bandwidth growth, i.e. 25%. The BBR paper can be read online at ACM
// Queue <https://queue.acm.org/detail.cfm?id=3022184>.
//
// WARNING: This algorithm is still experimental and we SHOULD NOT rely on
// it until we have gathered a better understanding of how it performs.
//
// TODO(bassosimone): more research is needed!
func stableAccordingToBBR(prev, cur, rtt float64, elapsed time.Duration) bool {
	return (elapsed.Seconds()*1000.0) >= (10.0*rtt) && cur >= prev &&
		(cur-prev) < (0.25*prev)
}

// warnAndClose emits a warning |message| and then closes the HTTP connection
// using the |writer| http.ResponseWriter.
func warnAndClose(writer http.ResponseWriter, message string) {
	ErrorLogger.Warn(message)
	writer.Header().Set("Connection", "Close")
	writer.WriteHeader(http.StatusBadRequest)
}

// makePadding generates a |size|-bytes long string containing random
// characters extracted from the [A-Za-z] set.
func makePadding(size int) string {
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
	return string(data)
}

// downloadLoop loops until the download is complete. |conn| is the WebSocket
// connection. |fp| is a os.File bound to the same descriptor of |conn| that
// allows us to extract BBR stats on Linux systems. |resultfp| is the file in
// which the measurements will be written.
func downloadLoop(conn *websocket.Conn, fp *os.File, resultfp *resultsfile) {
	ErrorLogger.Debug("Generating random buffer")
	const bufferSize = 1 << 13
	padding := makePadding(bufferSize)
	ErrorLogger.Debug("Start sending data to client")
	t0 := time.Now()
	count := float64(0.0)
	maxBandwidth := float64(0.0)
	tcpInfoWarnEmitted := false
	bbrWarnEmitted := false
	for {
		t := time.Now()
		elapsed := t.Sub(t0)
		measurement := Measurement{
			Elapsed:  elapsed.Seconds(),
			NumBytes: count,
		}
		stoppable := false
		if fp != nil {
			bw, rtt, err := bbr.GetMaxBandwidthAndMinRTT(fp)
			if err == nil {
				measurement.BBRInfo = &BBRInfo{
					MaxBandwidth: bw,
					MinRTT:       rtt,
				}
				ErrorLogger.Infof("Elapsed: %f s; BW: %f bit/s; RTT: %f ms",
						elapsed.Seconds(), bw, rtt)
				stoppable = stableAccordingToBBR(maxBandwidth, bw, rtt, elapsed)
				maxBandwidth = bw
			} else if bbrWarnEmitted == false {
				ErrorLogger.WithError(err).Warn("Cannot get BBR metrics")
				bbrWarnEmitted = true
			}
			metrics, err := tcpinfox.GetTCPInfo(fp)
			if err == nil {
				measurement.TCPInfo = &metrics
			} else if tcpInfoWarnEmitted == false {
				ErrorLogger.WithError(err).Warn("Cannot get TCP_INFO metrics")
				tcpInfoWarnEmitted = true
			}
		}
		if err := resultfp.WriteMeasurement(measurement, "server"); err != nil {
			ErrorLogger.WithError(err).Warn("Cannot save measurement on disk")
			return
		}
		conn.SetWriteDeadline(time.Now().Add(defaultTimeout))
		measurement.Padding = padding // after we've serialised on disk
		data, err := json.Marshal(measurement)
		if err != nil {
			ErrorLogger.WithError(err).Warn("Cannot serialise measurement message")
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			ErrorLogger.WithError(err).Warn("Cannot send serialised measurement message")
			return
		}
		if stoppable {
			ErrorLogger.Info("It seems we can stop the download earlier")
			// Disable breaking out of the loop for now because we've determined
			// that the best course of action is actually to run for 10 seconds to
			// gather enough data to refine the "stop early" algorithm.
		}
		if time.Now().Sub(t0) >= defaultDuration {
			break
		}
		count += float64(len(data))
	}
	ErrorLogger.Debug("Download test complete")
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
	ErrorLogger.Debug("Processing query string")
	meta := make(metadata)
	initMetadata(&meta, conn.LocalAddr().String(),
		conn.RemoteAddr().String(), request.URL.Query(), "download")
	resultfp, err := newResultsfile()
	if err != nil {
		ErrorLogger.WithError(err).Warn("Cannot open results file")
		return
	}
	defer resultfp.Close()
	if err := resultfp.WriteMetadata(meta); err != nil {
		ErrorLogger.WithError(err).Warn("Cannot write metadata to results file")
		return
	}
	fp := fdcache.GetAndForgetFile(conn.UnderlyingConn())
	// Implementation note: in theory fp SHOULD always be non-nil because
	// now we always register the fp bound to a net.TCPConn. However, in
	// some weird cases it MAY happen that the cache pruning mechanism will
	// remove the fp BEFORE we can steal it. For this reason, I've decided
	// to program defensively rather than calling panic() here.
	if fp != nil {
		defer fp.Close()  // We own `fp` and we must close it when done
		err = bbr.Enable(fp)
		if err != nil {
			ErrorLogger.WithError(err).Warn("Cannot enable BBR")
			// FALLTHROUGH
		}
	}
	// TODO(bassosimone): an error before this point means that the *os.File
	// will stay in cache until the cache pruning mechanism is triggered. This
	// should be a small amount of seconds. If Golang does not call shutdown(2)
	// and close(2), we'll end up keeping sockets that caused an error in the
	// code above (e.g. because the handshake was not okay) alive for the time
	// in which the corresponding *os.File is kept in cache.
	conn.SetReadLimit(MinMaxMessageSize)
	defer conn.Close()
	downloadLoop(conn, fp, resultfp)
	ErrorLogger.Debug("Closing the WebSocket connection")
	conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(
		websocket.CloseNormalClosure, ""), time.Now().Add(defaultTimeout))
}
