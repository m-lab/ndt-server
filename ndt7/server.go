package ndt7

import (
	"crypto/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/apex/log"
	"github.com/gorilla/websocket"
)

// defaultDuration is the default duration of a subtest in nanoseconds.
const defaultDuration = 10 * time.Second

// maxDuration is the maximum duration of a subtest in seconds
const maxDuration = 600

// DownloadHandler handles a download subtest from the server side.
type DownloadHandler struct {
	Upgrader websocket.Upgrader
}

// Handle handles the download subtest.
func (dl DownloadHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	log.Debug("Processing query string")
	duration := defaultDuration
	{
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
	for running := true; running; {
		select {
		case t := <-ticker.C:
			// TODO(bassosimone): here we should also include tcp_info data
			measurement := Measurement{
				Elapsed:  t.Sub(t0).Nanoseconds(),
				NumBytes: count,
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
