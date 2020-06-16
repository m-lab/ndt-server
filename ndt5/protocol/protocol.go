package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path"
	"time"

	"github.com/gorilla/websocket"

	"github.com/m-lab/ndt-server/ndt5/web100"
	"github.com/m-lab/ndt-server/netx"
)

var verbose = flag.Bool("ndt5.protocol.verbose", false, "Print the contents of every message to the log")

// MessageType is the full set opf NDT protocol messages we understand.
type MessageType byte

const (
	// MsgUnknown is the zero-value of MessageType and it is the message type to
	// return under error conditions or when the message is malformed.
	MsgUnknown MessageType = iota
	// SrvQueue signals how long a client should wait.
	SrvQueue
	// MsgLogin is used for signalling capabilities.
	MsgLogin
	// TestPrepare indicates that the server is getting ready to run a test.
	TestPrepare
	// TestStart indicates prepapartion is complete and the test is about to run.
	TestStart
	// TestMsg is used for communication during a test.
	TestMsg
	// TestFinalize is the last message a test sends.
	TestFinalize
	// MsgError is sent when an error occurs.
	MsgError
	// MsgResults sends test results.
	MsgResults
	// MsgLogout is used to logout.
	MsgLogout
	// MsgWaiting is used for queue management.
	MsgWaiting
	// MsgExtendedLogin is used to signal advanced capabilities.
	MsgExtendedLogin
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
// allow non-websocket-based NDT tests in support of plain TCP clients.
type Connection interface {
	ReadMessage() (_ int, p []byte, err error) // The first value in the returned tuple should be ignored. It is included in the API for websocket.Conn compatibility.
	ReadBytes() (count int64, err error)
	WriteMessage(messageType int, data []byte) error
	FillUntil(t time.Time, buffer []byte) (bytesWritten int64, err error)
	ServerIPAndPort() (string, int)
	ClientIPAndPort() (string, int)
	Close() error
	UUID() string
	String() string
	Messager() Messager
}

var badUUID = "ERROR_DISCOVERING_UUID"

// UUIDToFile converts a UUID into a newly-created open file with the extension '.json'.
func UUIDToFile(dir, uuid string) (*os.File, error) {
	if uuid == badUUID {
		f, err := ioutil.TempFile(dir, badUUID+"*.json")
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

// measurer allows all types of connections to embed this struct and be measured
// in the same way. It also means that we have to write the complicated
// measurement code at most once.
type measurer struct {
	summaryC                 <-chan *web100.Metrics
	cancelMeasurementContext context.CancelFunc
}

// newMeasurer creates a measurer struct with sensible and safe defaults.
func newMeasurer() *measurer {
	// We want the channel to be closed by default, not nil. A read on a closed
	// channel returns immediately, while a read on a nil channel blocks forever.
	c := make(chan *web100.Metrics)
	close(c)
	return &measurer{
		summaryC: c,
		// We want the cancel function to always be safe to call.
		cancelMeasurementContext: func() {},
	}
}

// StartMeasuring starts a polling measurement goroutine using ci and runs until
// the ctx expires.
func (m *measurer) StartMeasuring(ctx context.Context, ci netx.ConnInfo) {
	var newctx context.Context
	newctx, m.cancelMeasurementContext = context.WithCancel(ctx)
	m.summaryC = web100.MeasureViaPolling(newctx, ci)
}

// StopMeasuring stops the measurement process and returns the collected
// measurements. The measurement process can also be stopped by cancelling the
// context that was passed in to StartMeasuring().
func (m *measurer) StopMeasuring() (*web100.Metrics, error) {
	m.cancelMeasurementContext() // Start the channel close process.

	summary := <-m.summaryC
	if summary == nil {
		return nil, errors.New("No data returned from web100.MeasureViaPolling due to nil")
	}
	return summary, nil
}

// wsConnection wraps a websocket connection to allow it to be used as a
// Connection.
type wsConnection struct {
	*websocket.Conn
	*measurer
}

// AdaptWsConn turns a websocket Connection into a struct which implements both Measurer and Connection
func AdaptWsConn(ws *websocket.Conn) MeasuredConnection {
	return &wsConnection{Conn: ws, measurer: newMeasurer()}
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
	ci := netx.ToConnInfo(ws.UnderlyingConn())
	ws.measurer.StartMeasuring(ctx, ci)
}

func (ws *wsConnection) UUID() string {
	ci := netx.ToConnInfo(ws.UnderlyingConn())
	id, err := ci.GetUUID()
	if err != nil {
		log.Println("Could not discover UUID:", err)
		// TODO: increment a metric
		return badUUID
	}
	return id
}

func (ws *wsConnection) ServerIPAndPort() (string, int) {
	localAddr := netx.ToTCPAddr(ws.UnderlyingConn().LocalAddr())
	return localAddr.IP.String(), localAddr.Port
}

func (ws *wsConnection) ClientIPAndPort() (string, int) {
	remoteAddr := netx.ToTCPAddr(ws.UnderlyingConn().RemoteAddr())
	return remoteAddr.IP.String(), remoteAddr.Port
}

// ReadBytes reads some bytes and discards them. This method is in service of
// the c2s test.
func (ws *wsConnection) ReadBytes() (int64, error) {
	var count int64
	_, buff, err := ws.ReadMessage()
	if buff != nil {
		count = int64(len(buff))
	}
	return count, err
}

func (ws *wsConnection) String() string {
	return ws.LocalAddr().String() + "<=WS(S),JSON=>" + ws.RemoteAddr().String()
}

func (ws *wsConnection) Messager() Messager {
	return JSON.Messager(ws)
}

// netConnection is a utility struct that allows us to use OS sockets and
// Websockets using the same set of methods. Its second element is a Reader
// because we want to allow the input channel to be buffered.
type netConnection struct {
	net.Conn
	*measurer
	input     io.Reader
	c2sBuffer []byte
	encoding  Encoding
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

func (nc *netConnection) ReadBytes() (bytesRead int64, err error) {
	n, err := nc.input.Read(nc.c2sBuffer)
	return int64(n), err
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
	ci := netx.ToConnInfo(nc.Conn)
	nc.measurer.StartMeasuring(ctx, ci)
}

func (nc *netConnection) UUID() string {
	ci := netx.ToConnInfo(nc.Conn)
	if ci == nil {
		log.Println("Connection is not a TCPConn")
		return badUUID
	}
	id, err := ci.GetUUID()
	if err != nil {
		log.Println("Could not discover UUID")
		// TODO: increment a metric
		return badUUID
	}
	return id
}

func (nc *netConnection) ServerIPAndPort() (string, int) {
	localAddr := netx.ToTCPAddr(nc.LocalAddr())
	return localAddr.IP.String(), localAddr.Port
}

func (nc *netConnection) ClientIPAndPort() (string, int) {
	remoteAddr := netx.ToTCPAddr(nc.RemoteAddr())
	return remoteAddr.IP.String(), remoteAddr.Port
}

func (nc *netConnection) String() string {
	return nc.LocalAddr().String() + "<=PLAIN," + nc.encoding.String() + "=>" + nc.RemoteAddr().String()
}

func (nc *netConnection) SetEncoding(e Encoding) {
	nc.encoding = e
}

func (nc *netConnection) Messager() Messager {
	return nc.encoding.Messager(nc)
}

// MeasuredFlexibleConnection allows a MeasuredConnection to switch between TLV or JSON encoding.
type MeasuredFlexibleConnection interface {
	MeasuredConnection
	SetEncoding(Encoding)
}

// AdaptNetConn turns a non-WS-based TCP connection into a protocol.MeasuredConnection that can have its encoding set on the fly.
func AdaptNetConn(conn net.Conn, input io.Reader) MeasuredFlexibleConnection {
	return &netConnection{Conn: conn, measurer: newMeasurer(), input: input, c2sBuffer: make([]byte, 8192)}
}

// ReadTLVMessage reads a single NDT message out of the connection.
func ReadTLVMessage(ws Connection, expectedTypes ...MessageType) ([]byte, MessageType, error) {
	_, inbuff, err := ws.ReadMessage()
	if err != nil {
		return nil, MsgUnknown, err
	}
	if len(inbuff) < 3 {
		return nil, MsgUnknown, errors.New("Message is too short")
	}
	foundType := false
	for _, t := range expectedTypes {
		foundType = foundType || (MessageType(inbuff[0]) == t)
	}
	if !foundType {
		return nil, MessageType(inbuff[0]), fmt.Errorf("Read wrong message type. Wanted one of %v, got %q", expectedTypes, MessageType(inbuff[0]))
	}
	// Verify that the expected length matches the given data.
	expectedLen := int(inbuff[1])<<8 + int(inbuff[2])
	if expectedLen != len(inbuff[3:]) {
		return nil, MessageType(inbuff[0]), fmt.Errorf("Message length (%d) does not match length of data received (%d)",
			expectedLen, len(inbuff[3:]))
	}
	return inbuff[3:], MessageType(inbuff[0]), nil
}

// WriteTLVMessage write a single NDT message to the connection.
func WriteTLVMessage(ws Connection, msgType MessageType, message string) error {
	msgBytes := []byte(message)
	if *verbose {
		log.Printf("%s is getting sent a TLV of: %s, %d, %q\n", ws.String(), msgType.String(), len(msgBytes), message)
	}
	outbuff := make([]byte, 3+len(msgBytes))
	outbuff[0] = byte(msgType)
	outbuff[1] = byte((len(msgBytes) >> 8) & 0xFF)
	outbuff[2] = byte(len(msgBytes) & 0xFF)
	for i := range msgBytes {
		outbuff[i+3] = msgBytes[i]
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
	jsonString, _, err := ReadTLVMessage(ws, expectedType)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(jsonString, &message)
	if err != nil {
		return &JSONMessage{Msg: string(jsonString)}, err
	}
	return message, nil
}

// SendJSONMessage writes a single NDT message in JSON format.
func SendJSONMessage(msgType MessageType, msg string, ws Connection) error {
	message := &JSONMessage{Msg: msg}
	return WriteTLVMessage(ws, msgType, message.String())
}
