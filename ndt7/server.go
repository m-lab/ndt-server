package ndt7

import (
	"crypto/rand"
	"net/http"
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
// of the test (expressed as a time.Duration).
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
func stableAccordingToBBR(prev, cur, rtt float64, elapsed time.Duration) bool {
	return elapsed >= (10.0*time.Duration(rtt)*time.Microsecond) &&
		cur >= prev && (cur-prev) < (0.25*prev)
}

// Handle handles the download subtest.
func (dl DownloadHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	log.Debug("Processing query string")
	duration := defaultDuration
	s := request.URL.Query().Get("duration")
	if s != "" {
		value, err := strconv.Atoi(s)
		if err != nil || value < 0 || value > maxDuration {
			log.Warn("The duration option has an invalid value")
			writer.Header().Set("Connection", "Close")
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		duration = time.Second * time.Duration(value)
	}
	adaptive := false
	s = request.URL.Query().Get("adaptive")
	if s != "" {
		value, err := strconv.ParseBool(s)
		if err != nil {
			log.Warn("The adaptive option has an invalid value")
			writer.Header().Set("Connection", "Close")
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		adaptive = value
	}
	log.Debug("Upgrading to WebSockets")
	if request.Header.Get("Sec-WebSocket-Protocol") != SecWebSocketProtocol {
		log.Warn("Missing Sec-WebSocket-Protocol in request")
		writer.Header().Set("Connection", "Close")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", SecWebSocketProtocol)
	conn, err := dl.Upgrader.Upgrade(writer, request, headers)
	if err != nil {
		log.WithError(err).Warn("upgrader.Upgrade() failed")
		return
	}
	// TODO(bassosimone): currently we're leaking filedesc cache entries if we
	// error out before this point. Because we have concluded that the cache
	// cannot grow indefinitely, this is probably not a priority.
	fd, err := bbr.ExtractFd(conn.LocalAddr().String())
	if err != nil {
		log.WithError(err).Warnf("Cannot get fd for: %s", conn.LocalAddr().String())
		// Continue processing. The |fd| will be invalid in this case but the
		// code below consider the case where |fd| is -1.
	}
	conn.SetReadLimit(MinMaxMessageSize)
	defer conn.Close()
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
	ticker := time.NewTicker(MinMeasurementInterval)
	defer ticker.Stop()
	t0 := time.Now()
	count := int64(0)
	bandwidth := float64(0)
	for running := true; running; {
		select {
		// TODO(bassosimone): I am confused by some experiments that I am running
		// where the RTT is > 0.25 s. In such cases, I do not see notifications
		// sent from the server to the client at the beginning of the connection
		// lifetime. Observe for example the following two seconds gap:
		//
		// +--------------+--------------------+----------+
		// | elapsed (ms) | bandwidth (Gbit/s) | RTT (ms) |
		// +--------------+--------------------+----------+
		// |          250 |              0.019 |  499.994 |
		// |         2250 |              0.035 |  499.994 |
		// |         2750 |              0.059 |  499.989 |
		// +--------------+--------------------+----------+
		//
		// I was actually (correctly?) expecting an event every 250 ms.
		//
		// What is going on? Can we observe the same issue on the server side
		// or is this somehow a client side artifact?
		case t := <-ticker.C:
			// TODO(bassosimone): here we should also include tcp_info data
			elapsed := t.Sub(t0)
			measurement := Measurement{
				Elapsed:  elapsed.Nanoseconds(),
				NumBytes: count,
			}
			if fd != -1 {
				// TODO(bassosimone): I am seeing cases in the logs where either at
				// the beginning of the connection, or after some time, I cannot get
				// anymore BBR info because of a EBADF error. Trying to understand
				// why this happens and whether it's specific of a specific Linux
				// kernel or related to some other feature is probably needed before
				// calling this code safe to be used in production.
				bw, rtt, err := bbr.GetBandwidthAndRTT(fd)
				if err == nil {
					measurement.BBRInfo = &BBRInfo{
						Bandwidth: bw,
						RTT:       rtt,
					}
					log.Infof("BW: %f bytes/s; RTT: %f usec", bw, rtt)
					stoppable := stableAccordingToBBR(bandwidth, bw, rtt, elapsed)
					if stoppable && adaptive {
						log.Info("It seems we can stop the download earlier")
						running = false
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
		default: // Not ticking, just send more data
			if time.Now().Sub(t0) >= duration {
				running = false
				break
			}
			conn.SetWriteDeadline(time.Now().Add(defaultTimeout))
			if err := conn.WritePreparedMessage(buffer); err != nil {
				log.WithError(err).Warn("cannot send data message")
				return
			}
			count += bufferSize
		}
	}
	log.Debug("Closing the WebSocket connection")
	conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(
		websocket.CloseNormalClosure, ""), time.Now().Add(defaultTimeout))
}
