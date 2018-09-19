package ndt7

import (
	"crypto/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/apex/log"
	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-cloud/bbr"
)

// defaultDuration is the default duration of a subtest in nanoseconds.
const defaultDuration = 10 * time.Second

// maxDuration is the maximum duration of a subtest in seconds
const maxDuration = 30

// DownloadHandler handles a download subtest from the server side.
type DownloadHandler struct {
	Upgrader websocket.Upgrader
}

// stableAccordingToBBR returns true when we can stop the current download
// test based on |prev|, the previous BBR bandwidth sample, |cur| the
// current BBR bandwidth sample, |rtt|, the BBR measured RTT (in
// microsecond), and |elapsed|, the elapsed time since the beginning
// of the test (expressed as a time.Duration). The bandwidth is measured
// in bytes per second.
//
// This algorithm runs every 0.25 seconds. Empirically, it is know that
// BBR requires some RTTs to converge. We are using 10 RTTs as a reasonable
// upper bound. Before 10 RTTs have elapsed, we do not check whether the
// bandwidth has stopped growing. After 10 RTTs have elapsed, we call
// the connection stable when the bandwidth measured by BBR does not
// grow of more than 25% between two 0.25 second periods.
//
// We use the same percentage used by the BBR paper to characterize the
// bandwidth growth, i.e. 25%. The BBR paper can be read online at ACM
// Queue <https://queue.acm.org/detail.cfm?id=3022184>.
//
// WARNING: This algorithm is still experimental and we SHOULD NOT rely on
// it until we have gathered a better understanding of how it performs.
//
// TODO(bassosimone): more research is needed!
func stableAccordingToBBR(prev, cur, rtt float64, elapsed time.Duration) bool {
	return (elapsed.Seconds()*1000*1000) >= (10.0*rtt) && cur >= prev &&
		(cur-prev) < (0.25*prev)
}

// getDuration gets the duration from the |request|'s query string. If no
// duration option is defined in the query string, the default is returned. In
// case of error processing the query string, an error is returned.
func getDuration(request *http.Request) (time.Duration, error) {
	duration := defaultDuration
	s := request.URL.Query().Get("duration")
	if s != "" {
		value, err := strconv.Atoi(s)
		if err != nil || value < 0 || value > maxDuration {
			return 0, err
		}
		duration = time.Second * time.Duration(value)
	}
	return duration, nil
}

// getAdaptive gets the adaptive option from the |request|'s query string. If
// no option is defined in the query string, then default value is returned. In
// case of error processing the query string, an error is returned.
func getAdaptive(request *http.Request) (bool, error) {
	adaptive := false
	s := request.URL.Query().Get("adaptive")
	if s != "" {
		value, err := strconv.ParseBool(s)
		if err != nil {
			return false, err
		}
		adaptive = value
	}
	return adaptive, nil
}

// warnAndClose emits a warning |message| and then closes the HTTP connection
// using the |writer| http.ResponseWriter.
func warnAndClose(writer http.ResponseWriter, message string) {
	log.Warn(message)
	writer.Header().Set("Connection", "Close")
	writer.WriteHeader(http.StatusBadRequest)
}

// downloadLoop loops until the download is complete. |conn| is the WebSocket
// connection. |fp| is a os.File bound to the same descriptor of |conn| that
// allows us to extract BBR stats on Linux systems. |adaptive| tells us whether
// we may interrupt early the download if BBR stats indicate that BBR has a
// stable max-bandwidth estimate. |duration| is the expected duration of the
// test, which may be shorter if |adaptive| is true.
func downloadLoop(conn *websocket.Conn, fp *os.File, adaptive bool, duration time.Duration) {
	log.Debug("Generating random buffer")
	const bufferSize = 1 << 13
	data := make([]byte, bufferSize)
	rand.Read(data)
	buffer, err := websocket.NewPreparedMessage(websocket.BinaryMessage, data)
	if err != nil {
		log.WithError(err).Warn("websocket.NewPreparedMessage() failed")
		return
	}
	log.Debug("Start sending data to client")
	t0 := time.Now()
	last := t0
	count := int64(0)
	bandwidth := float64(0)
	for {
		t := time.Now()
		if t.Sub(last) >= MinMeasurementInterval {
			// TODO(bassosimone): here we should also include tcp_info data
			elapsed := t.Sub(t0)
			measurement := Measurement{
				Elapsed:  elapsed.Nanoseconds(),
				NumBytes: count,
			}
			if fp != nil {
				bw, rtt, err := bbr.GetBandwidthAndRTT(fp)
				if err == nil {
					measurement.BBRInfo = &BBRInfo{
						Bandwidth: bw,
						RTT:       rtt,
					}
					log.Infof("BW: %f bytes/s; RTT: %f usec", bw, rtt)
					stoppable := stableAccordingToBBR(bandwidth, bw, rtt, elapsed)
					if stoppable && adaptive {
						log.Info("It seems we can stop the download earlier")
						break
					}
					bandwidth = bw
				} else {
					log.WithError(err).Warn("Cannot get BBR info")
				}
			}
			conn.SetWriteDeadline(time.Now().Add(defaultTimeout))
			if err := conn.WriteJSON(&measurement); err != nil {
				log.WithError(err).Warn("Cannot send measurement message")
				return
			}
			last = t
		}
		if time.Now().Sub(t0) >= duration {
			break
		}
		conn.SetWriteDeadline(time.Now().Add(defaultTimeout))
		if err := conn.WritePreparedMessage(buffer); err != nil {
			log.WithError(err).Warn("cannot send data message")
			return
		}
		count += bufferSize
	}
	log.Debug("Download test complete")
}

// Handle handles the download subtest.
func (dl DownloadHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	log.Debug("Processing query string")
	duration, err := getDuration(request)
	if err != nil {
		warnAndClose(writer, "The duration option has an invalid value")
		return
	}
	adaptive, err := getAdaptive(request)
	if err != nil {
		warnAndClose(writer, "The adaptive option has an invalid value")
		return
	}
	log.Debug("Upgrading to WebSockets")
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
	fp := bbr.GetAndForgetFile(conn.UnderlyingConn())
	if fp != nil {
		defer fp.Close()  // We own `fp` and we must close it when done
	}
	// TODO(bassosimone): an error before this point means that the *os.File
	// will stay in cache until the cache pruning mechanism is triggered. This
	// should be a small amount of seconds. If Golang does not call shutdown(2)
	// and close(2), we'll end up keeping sockets that caused an error in the
	// code above (e.g. because the handshake was not okay) alive for the time
	// in which the corresponding *os.File is kept in cache.
	conn.SetReadLimit(MinMaxMessageSize)
	defer conn.Close()
	downloadLoop(conn, fp, adaptive, duration)
	log.Debug("Closing the WebSocket connection")
	conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(
		websocket.CloseNormalClosure, ""), time.Now().Add(defaultTimeout))
}
