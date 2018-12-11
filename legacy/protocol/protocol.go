package protocol

import (
	"encoding/json"
	"fmt"

	"github.com/gorilla/websocket"
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

// Connection is a general system over which we might be able to read an NDT message.
// It contains a subset of the methods of websocket.Conn, in order to allow non-websocket-based NDT tests in support of legacy clients.
// Every websocket.Conn already implements Connection.
type Connection interface {
	ReadMessage() (_ int, p []byte, err error) // The first value in the returned tuple should be ignored. It is included in the API for websocket.Conn compatibility.
	WriteMessage(messageType int, data []byte) error
}

// ReadMessage reads a single NDT message out of the connection.
func ReadMessage(ws Connection, expectedType MessageType) ([]byte, error) {
	_, inbuff, err := ws.ReadMessage()
	if err != nil {
		return nil, err
	}
	if MessageType(inbuff[0]) != expectedType {
		return nil, fmt.Errorf("Read wrong message type. Wanted 0x%x, got 0x%x", expectedType, inbuff[0])
	}
	// Verify that the expected length matches the given data.
	expectedLen := int(inbuff[1])<<8 + int(inbuff[2])
	if expectedLen != len(inbuff[3:]) {
		return nil, fmt.Errorf("Message length (%d) does not match length of data received (%d)",
			expectedLen, len(inbuff[3:]))
	}
	return inbuff[3:], nil
}

// WriteMessage write a single NDT message to the connection.
func WriteMessage(ws Connection, msgType MessageType, msg fmt.Stringer) error {
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
	jsonString, err := ReadMessage(ws, expectedType)
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
	return WriteMessage(ws, msgType, message)
}
