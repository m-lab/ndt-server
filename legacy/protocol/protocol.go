package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path"
	"reflect"
	"time"

	"github.com/m-lab/ndt-server/fdcache"
	"github.com/m-lab/uuid"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/legacy/web100"
)

// MessageType is the full set opf NDT protocol messages we understand.
type MessageType byte

const (
	// SrvQueue signals how long a client should wait.
	SrvQueue = MessageType(1)
	// MsgLogin is used for signalling capabilities.
	MsgLogin = MessageType(2)
	// TestPrepare indicates that the server is getting ready to run a test.
	TestPrepare = MessageType(3)
	// TestStart indicates prepapartion is complete and the test is about to run.
	TestStart = MessageType(4)
	// TestMsg is used for communication during a test.
	TestMsg = MessageType(5)
	// TestFinalize is the last message a test sends.
	TestFinalize = MessageType(6)
	// MsgError is sent when an error occurs.
	MsgError = MessageType(7)
	// MsgResults sends test results.
	MsgResults = MessageType(8)
	// MsgLogout is used to logout.
	MsgLogout = MessageType(9)
	// MsgWaiting is used for queue management.
	MsgWaiting = MessageType(10)
	// MsgExtendedLogin is used to signal advanced (now required) capabilities.
	MsgExtendedLogin = MessageType(11)
)

func (m MessageType) String() string {
	switch m {
	case SrvQueue:
		return "SrvQueue"
	case MsgLogin:
		return "MsgLogin"
	case TestPrepare:
		return "TestPrepare"
	case TestStart:
		return "TestStart"
	case TestMsg:
		return "TestMsg"
	case TestFinalize:
		return "TestFinalize"
	case MsgError:
		return "MsgError"
	case MsgResults:
		return "MsgResults"
	case MsgLogout:
		return "MsgLogout"
	case MsgWaiting:
		return "MsgWaiting"
	case MsgExtendedLogin:
		return "MsgExtendedLogin"
	default:
		return fmt.Sprintf("UnknownMessage(0x%X)", byte(m))
	}
}

// Connection is a general system over which we might be able to read an NDT
// message. It contains a subset of the methods of websocket.Conn, in order to
// allow non-websocket-based NDT tests in support of legacy clients, along with
// the new methods "DrainUntil" and "FillUntil".
type Connection interface {
	ReadMessage() (_ int, p []byte, err error) // The first value in the returned tuple should be ignored. It is included in the API for websocket.Conn compatibility.
	WriteMessage(messageType int, data []byte) error
	DrainUntil(t time.Time) (bytesRead int64, err error)
	FillUntil(t time.Time, buffer []byte) (bytesWritten int64, err error)
	SetReadDeadline(t time.Time) error
	ServerIP() string
	ClientIP() string
	Close() error
	UUID() string
}

var badUUID = "BAD_UUID"

// UUIDToFile converts a UUID into a newly-created open file with the extension '.json'.
func UUIDToFile(dir, uuid string) (*os.File, error) {
	if uuid == badUUID {
		f, err := ioutil.TempFile(dir, badUUID+"XXXXXX.json")
		if err != nil {
			log.Println("Could not create filename for data")
			return nil, err
		}
		return f, nil
	}
	return os.Create(path.Join(dir, uuid+".json"))
}

// Measurable things can be measured over a given timeframe.
type Measurable interface {
	StartMeasuring(ctx context.Context)
	StopMeasuring() (*web100.Metrics, error)
}

// MeasuredConnection is a connection which can also be measured.
type MeasuredConnection interface {
	Connection
	Measurable
}

// The measurer struct is a hack to ensure that we only have to write the
// complicated measurement code at most once.
type measurer struct {
	measurements             chan *web100.Metrics
	cancelMeasurementContext context.CancelFunc
}

func (m *measurer) StartMeasuring(ctx context.Context, fd *os.File) {
	m.measurements = make(chan *web100.Metrics)
	var newctx context.Context
	newctx, m.cancelMeasurementContext = context.WithCancel(ctx)
	go web100.MeasureViaPolling(newctx, fd, m.measurements)
}

func (m *measurer) StopMeasuring() (*web100.Metrics, error) {
	m.cancelMeasurementContext()
	info, ok := <-m.measurements
	if !ok {
		return nil, errors.New("No data")
	}
	return info, nil
}

// wsConnection wraps a websocket connection to allow it to be used as a
// Connection.
type wsConnection struct {
	*websocket.Conn
	*measurer
}

// AdaptWsConn turns a websocket Connection into a struct which implements both Measurer and Connection
func AdaptWsConn(ws *websocket.Conn) MeasuredConnection {
	return &wsConnection{Conn: ws, measurer: &measurer{}}
}

func (ws *wsConnection) DrainUntil(t time.Time) (bytesRead int64, err error) {
	for time.Now().Before(t) {
		_, buffer, err := ws.ReadMessage()
		if err != nil {
			return bytesRead, err
		}
		bytesRead += int64(len(buffer))
	}
	return bytesRead, nil
}

func (ws *wsConnection) FillUntil(t time.Time, bytes []byte) (bytesWritten int64, err error) {
	messageToSend, err := websocket.NewPreparedMessage(websocket.BinaryMessage, bytes)
	if err != nil {
		return 0, err
	}
	for time.Now().Before(t) {
		err := ws.WritePreparedMessage(messageToSend)
		if err != nil {
			return bytesWritten, err
		}
		bytesWritten += int64(len(bytes))
	}
	return bytesWritten, nil
}

func (ws *wsConnection) StartMeasuring(ctx context.Context) {
	ws.measurer.StartMeasuring(ctx, fdcache.GetAndForgetFile(ws.UnderlyingConn()))
}

func (ws *wsConnection) UUID() string {
	id, err := uuid.FromTCPConn(ws.UnderlyingConn().(*net.TCPConn))
	if err != nil {
		log.Println("Could not discover UUID")
		// TODO: increment a metric
		return badUUID
	}
	return id
}

func (ws *wsConnection) ServerIP() string {
	return ws.UnderlyingConn().LocalAddr().String()
}

func (ws *wsConnection) ClientIP() string {
	return ws.UnderlyingConn().RemoteAddr().String()
}

// netConnection is a utility struct that allows us to use OS sockets and
// Websockets using the same set of methods. Its second element is a Reader
// because we want to allow the input channel to be buffered.
type netConnection struct {
	net.Conn
	*measurer
	input io.Reader
}

func (nc *netConnection) ReadMessage() (int, []byte, error) {
	firstThree := make([]byte, 3)
	_, err := nc.input.Read(firstThree)
	if err != nil {
		return 0, []byte{}, err
	}
	size := int64(firstThree[1])<<8 + int64(firstThree[2])
	bytes := make([]byte, size)
	_, err = nc.input.Read(bytes)
	return 0, append(firstThree, bytes...), err
}

func (nc *netConnection) WriteMessage(_messageType int, data []byte) error {
	// _messageType is ignored because it is meaningless for a net.Conn
	_, err := nc.Write(data)
	return err
}

func (nc *netConnection) DrainUntil(t time.Time) (bytesRead int64, err error) {
	buff := make([]byte, 8192)
	for time.Now().Before(t) {
		n, err := nc.Read(buff)
		if err != nil {
			return bytesRead, err
		}
		bytesRead += int64(n)
	}
	return bytesRead, nil
}

func (nc *netConnection) FillUntil(t time.Time, bytes []byte) (bytesWritten int64, err error) {
	for time.Now().Before(t) {
		n, err := nc.Write(bytes)
		if err != nil {
			return bytesWritten, err
		}
		bytesWritten += int64(n)
	}
	return bytesWritten, nil
}

func (nc *netConnection) StartMeasuring(ctx context.Context) {
	nc.measurer.StartMeasuring(ctx, fdcache.GetAndForgetFile(nc))
}

func (nc *netConnection) UUID() string {
	id, err := uuid.FromTCPConn(nc.Conn.(*net.TCPConn))
	if err != nil {
		log.Println("Could not discover UUID")
		// TODO: increment a metric
		return badUUID
	}
	return id
}

func (nc *netConnection) ServerIP() string {
	return nc.LocalAddr().String()
}

func (nc *netConnection) ClientIP() string {
	return nc.RemoteAddr().String()
}

// AdaptNetConn turns a non-WS-based TCP connection into a protocol.MeasuredConnection.
func AdaptNetConn(conn net.Conn, input io.Reader) MeasuredConnection {
	return &netConnection{Conn: conn, measurer: &measurer{}, input: input}
}

// ReadNDTMessage reads a single NDT message out of the connection.
func ReadNDTMessage(ws Connection, expectedType MessageType) ([]byte, error) {
	_, inbuff, err := ws.ReadMessage()
	if err != nil {
		return nil, err
	}
	if MessageType(inbuff[0]) != expectedType {
		return nil, fmt.Errorf("Read wrong message type. Wanted %q, got %q", expectedType, MessageType(inbuff[0]))
	}
	// Verify that the expected length matches the given data.
	expectedLen := int(inbuff[1])<<8 + int(inbuff[2])
	if expectedLen != len(inbuff[3:]) {
		return nil, fmt.Errorf("Message length (%d) does not match length of data received (%d)",
			expectedLen, len(inbuff[3:]))
	}
	return inbuff[3:], nil
}

// WriteNDTMessage write a single NDT message to the connection.
func WriteNDTMessage(ws Connection, msgType MessageType, msg fmt.Stringer) error {
	message := msg.String()
	outbuff := make([]byte, 3+len(message))
	outbuff[0] = byte(msgType)
	outbuff[1] = byte((len(message) >> 8) & 0xFF)
	outbuff[2] = byte(len(message) & 0xFF)
	for i := range message {
		outbuff[i+3] = message[i]
	}
	return ws.WriteMessage(websocket.BinaryMessage, outbuff)
}

// JSONMessage holds the JSON messages we can receive from the server. We
// only support the subset of the NDT JSON protocol that has two fields: msg,
// and tests.
type JSONMessage struct {
	Msg   string `json:"msg"`
	Tests string `json:"tests,omitempty"`
}

// String serializes the message to a string.
func (n *JSONMessage) String() string {
	b, _ := json.Marshal(n)
	return string(b)
}

// ReceiveJSONMessage reads a single NDT message in JSON format.
func ReceiveJSONMessage(ws Connection, expectedType MessageType) (*JSONMessage, error) {
	message := &JSONMessage{}
	jsonString, err := ReadNDTMessage(ws, expectedType)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(jsonString, &message)
	if err != nil {
		return nil, err
	}
	return message, nil
}

// SendJSONMessage writes a single NDT message in JSON format.
func SendJSONMessage(msgType MessageType, msg string, ws Connection) error {
	message := &JSONMessage{Msg: msg}
	return WriteNDTMessage(ws, msgType, message)
}

// SendMetrics sends all the required properties out along the NDT control channel.
func SendMetrics(metrics *web100.Metrics, ws Connection) error {
	v := reflect.ValueOf(*metrics)
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		name := t.Field(i).Name
		msg := fmt.Sprintf("%s: %v\n", name, v.Field(i).Interface())
		err := SendJSONMessage(TestMsg, msg, ws)
		if err != nil {
			return err
		}
	}
	return nil
}
