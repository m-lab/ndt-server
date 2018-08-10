package ndt7

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/apex/log"
	"github.com/gorilla/websocket"
)

// Client is a simplified NDT7 client.
type Client struct {
	Dialer websocket.Dialer
	URL    url.URL
}

// defaultTimeout is the default value of the I/O timeout.
const defaultTimeout = 1 * time.Second

// Download runs a NDT7 download test.
func (cl Client) Download() error {
	cl.URL.Path = DownloadURLPath
	log.Infof("Creating a WebSocket connection to: %s", cl.URL.String())
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", SecWebSocketProtocol)
	cl.Dialer.HandshakeTimeout = defaultTimeout
	conn, _, err := cl.Dialer.Dial(cl.URL.String(), headers)
	if err != nil {
		return err
	}
	conn.SetReadLimit(MinMaxMessageSize)
	defer conn.Close()
	t0 := time.Now()
	num := int64(0)
	ticker := time.NewTicker(MinMeasurementInterval)
	log.Info("Starting download")
	for {
		select {
		case t1 := <-ticker.C:
			// Print the current speed on the stdout because this gives us an idea
			// of how fast we are downloading from the remote server.
			v := (float64(num) * 8.0) / t1.Sub(t0).Seconds() / 1e06
			log.Infof("Client measurement: %02f Mbit/s", v)
			t0 = t1
			num = int64(0)
		default:
			// Just fallthrough
		}
		conn.SetReadDeadline(time.Now().Add(defaultTimeout))
		mtype, mdata, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				return err
			}
			break
		}
		num += int64(len(mdata))
		if mtype == websocket.TextMessage {
			// Unmarshal to verify that this message is correct JSON but do not
			// otherwise process the message's content.
			measurement := Measurement{}
			err := json.Unmarshal(mdata, &measurement)
			if err != nil {
				return err
			}
		}
	}
	log.Info("Download complete")
	return nil
}
