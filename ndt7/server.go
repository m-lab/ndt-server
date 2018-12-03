package ndt7

import (
	"context"
	"math/rand"
	"net/http"
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
// TODO(bassosimone): more research is needed! Therefore this function is
// currently not used. We're now collecting data useful to understand what
// is the best algorithm to stop. The existing code will stay here, in
// commented out form, to document what was our initial idea.
/*
func stableAccordingToBBR(prev, cur, rtt float64, elapsed time.Duration) bool {
	return (elapsed.Seconds()*1000.0) >= (10.0*rtt) && cur >= prev &&
		(cur-prev) < (0.25*prev)
}
*/

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

// startMeasuring runs the measurement loop. This runs in a separate goroutine
// and emits Measurement events on the returned channel. The goroutine will
// exit when (1) a fatal error occurs or (2) the maximum elapsed time for the
// download test expires. Because the goroutine has access to BBR stats (if BBR
// is available), then that's the right place to stop the test early. The rest
// of the download code is supposed to stop downloading when this goroutine will
// signal that we're done by closing the channel. The goroutine will not tell
// the test of the downloader whether an error occurred because closing it will
// log any error and closing the channel provides already enoug bits of info
// to synchronize this part of the downloader with the rest. The context param
// will be used by the outer loop to tell us when we need to stop early.
func startMeasuring(ctx context.Context, request *http.Request, conn *websocket.Conn) chan Measurement {
	dst := make(chan Measurement)
	go func() {
		defer close(dst) // make sure we close the channel when we leave
		ErrorLogger.Debug("Processing query string")
		meta := make(metadata)
		initMetadata(&meta, conn.LocalAddr().String(),
			conn.RemoteAddr().String(), request.URL.Query(), "download")
		resultfp, err := newResultsfile()
		if err != nil {
			ErrorLogger.WithError(err).Warn("Cannot open results file")
			return
		}
		ErrorLogger.Debug("Writing metadata on results file")
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
			defer fp.Close() // We own `fp` and we must close it when done
			err = bbr.Enable(fp)
			if err != nil {
				ErrorLogger.WithError(err).Warn("Cannot enable BBR")
				// FALLTHROUGH
			}
		}
		t0 := time.Now()
		ticker := time.NewTicker(MinMeasurementInterval)
		bbrWarnEmitted := false
		tcpInfoWarnEmitted := false
		ErrorLogger.Debug("Starting measurement loop")
		for {
			select {
			case <-ctx.Done():
				goto out
			case now := <-ticker.C:
				elapsed := now.Sub(t0)
				if elapsed > defaultDuration {
					ErrorLogger.Debug("Download run for enough time")
					goto out
				}
				measurement := Measurement{
					Elapsed: elapsed.Seconds(),
				}
				if fp != nil {
					bw, rtt, err := bbr.GetMaxBandwidthAndMinRTT(fp)
					if err == nil {
						measurement.BBRInfo = &BBRInfo{
							MaxBandwidth: bw,
							MinRTT:       rtt,
						}
						ErrorLogger.Infof("Elapsed: %f s; BW: %f bit/s; RTT: %f ms",
							elapsed.Seconds(), bw, rtt)
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
					goto out
				}
				dst <- measurement
				// TODO(bassosimone): when we've collected more data, this is the right
				// place to decide whether to close the channel based on BBR data.
			}
		}
	out:
		ErrorLogger.Debug("Stopping measurement loop")
	}()
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
