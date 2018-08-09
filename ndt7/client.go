package ndt7

import (
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/apex/log"
	"github.com/gorilla/websocket"
)

// Settings contains client settings. All settings are optional except for
// the Hostname, which cannot be autoconfigured at the moment.
type Settings struct {
	// This structure embeds options defined in the spec.
	Options
	// InsecureSkipTLSVerify can be used to disable certificate verification.
	InsecureSkipTLSVerify bool `json:"skip_tls_verify"`
	// InsecureNoTLS can be used to force using cleartext.
	InsecureNoTLS bool `json:"no_tls"`
	// Hostname is the hostname of the NDT7 server to connect to.
	Hostname string `json:"hostname"`
	// Port is the port of the NDT7 server to connect to.
	Port string `json:"port"`
}

// Client is a NDT7 client.
type Client struct {
	dialer websocket.Dialer
	url    url.URL
}

// NewClient creates a new client.
func NewClient(settings Settings) Client {
	cl := Client{}
	cl.dialer.HandshakeTimeout = defaultTimeout
	if settings.InsecureSkipTLSVerify {
		config := tls.Config{InsecureSkipVerify: true}
		cl.dialer.TLSClientConfig = &config
		log.Warn("Disabling TLS cerificate verification (INSECURE!)")
	}
	if settings.InsecureNoTLS {
		log.Warn("Using plain text WebSocket (INSECURE!)")
		cl.url.Scheme = "ws"
	} else {
		cl.url.Scheme = "wss"
	}
	if settings.Port != "" {
		ip := net.ParseIP(settings.Hostname)
		if ip == nil || len(ip) == 4 {
			cl.url.Host = settings.Hostname
			cl.url.Host += ":"
			cl.url.Host += settings.Port
		} else if len(ip) == 16 {
			cl.url.Host = "["
			cl.url.Host += settings.Hostname
			cl.url.Host += "]:"
			cl.url.Host += settings.Port
		} else {
			panic("IP address that is neither 4 nor 16 bytes long")
		}
	} else {
		cl.url.Host = settings.Hostname
	}
	query := cl.url.Query()
	if settings.Duration > 0 {
		query.Add("duration", strconv.Itoa(settings.Duration))
	}
	cl.url.RawQuery = query.Encode()
	return cl
}

// EvKey uniquely identifies an event.
type EvKey int

const (
	// LogEvent indicates an event containing a log message
	LogEvent = EvKey(iota)
	// MeasurementEvent indicates an event containing some measurements
	MeasurementEvent
	// FailureEvent indicates an event containing an error
	FailureEvent
)

// Event is the structure of a generic event
type Event struct {
	Key   EvKey       // Tells you the kind of the event
	Value interface{} // Opaque event value
}

// Severity indicates the severity of a log message
type Severity int

const (
	// LogWarning indicates a warning message
	LogWarning = Severity(iota)
	// LogInfo indicates an informational message
	LogInfo
	// LogDebug indicates a debug message
	LogDebug
)

// LogRecord is the structure of a log event
type LogRecord struct {
	Severity Severity // Message severity
	Message  string   // The message
}

// MeasurementRecord is the structure of a measurement event
type MeasurementRecord struct {
	Measurement      // The measurement
	IsLocal     bool `json:"is_local"` // Whether it is a local measurement
}

// FailureRecord is the structure of a failure event
type FailureRecord struct {
	Err error // The error that occurred
}

// defaultTimeout is the default value of the I/O timeout.
const defaultTimeout = 1 * time.Second

// Download runs a NDT7 download test.
func (cl Client) Download() error {
	log.Info("Creating a WebSocket connection")
	cl.url.Path = DownloadURLPath
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", SecWebSocketProtocol)
	conn, _, err := cl.dialer.Dial(cl.url.String(), headers)
	if err != nil {
		return err
	}
	conn.SetReadLimit(MinMaxMessageSize)
	defer conn.Close()
	log.Info("Starting download")
	for {
		conn.SetReadDeadline(time.Now().Add(defaultTimeout))
		mtype, mdata, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				return err
			}
			break
		}
		if mtype == websocket.TextMessage {
			// Unmarshaling to verify that this message is correct JSON
			measurement := Measurement{}
			err := json.Unmarshal(mdata, &measurement)
			if err != nil {
				return err
			}
			log.Infof("Server measurement: %s", mdata)
		}
	}
	log.Info("Download complete")
	return nil
}
