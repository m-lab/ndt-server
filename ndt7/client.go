package ndt7

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/apex/log"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/gorilla/websocket"
)

// minMeasurementInterval is the minimum value of the interval betwen
// two consecutive measurements performed by either party. An implementation
// MAY choose to close the connection if it is receiving too frequent
// Measurement messages from the other endpoint.
const minMeasurementInterval = 250 * time.Millisecond

// defaultTimeout is the default I/O timeout.
const defaultTimeout = 7 * time.Second

// Client is a simplified ndt7 client.
type Client struct {
	Dialer websocket.Dialer
	URL    url.URL
}

// Download runs a ndt7 download test.
func (cl Client) Download() error {
	cl.URL.Path = spec.DownloadURLPath
	log.Infof("Creating a WebSocket connection to: %s", cl.URL.String())
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	cl.Dialer.HandshakeTimeout = defaultTimeout
	conn, _, err := cl.Dialer.Dial(cl.URL.String(), headers)
	if err != nil {
		return err
	}
	conn.SetReadLimit(spec.MinMaxMessageSize)
	defer conn.Close()
	t0 := time.Now()
	num := int64(0.0)
	ticker := time.NewTicker(minMeasurementInterval)
	log.Info("Starting download")
	for {
		select {
		case t1 := <-ticker.C:
			mm := model.Measurement{
				AppInfo: &model.AppInfo{
					NumBytes: num,
				},
				Elapsed: t1.Sub(t0).Seconds(),
			}
			data, err := json.Marshal(mm)
			if err != nil {
				panic("cannot unmarshal JSON")
			}
			log.Infof("client: %s", data)
		default:
			// Just fallthrough
		}
		conn.SetReadDeadline(time.Now().Add(defaultTimeout))
		mtype, mdata, err := conn.ReadMessage()
		if err != nil {
			// Implementation note: "Expecting clients to `stick around` for no
			// benefit other than RFC-compliance is a losing proposition" (-pboothe).
			// See also <https://en.wikipedia.org/wiki/Byzantine_fault>.
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				log.WithError(err).Warn("did not receive a normal websocket closure")
				// FALLTHROUGH
			}
			break
		}
		num += int64(len(mdata))
		if mtype == websocket.TextMessage {
			// Unmarshal to verify that this message is correct JSON but do not
			// otherwise process the message's content.
			measurement := model.Measurement{}
			err := json.Unmarshal(mdata, &measurement)
			if err != nil {
				return err
			}
			log.Infof("server: %s", mdata)
		}
	}
	log.Info("Download complete")
	return nil
}
