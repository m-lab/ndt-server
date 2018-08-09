package legacy

import (
	"encoding/json"
	"fmt"

	"github.com/gorilla/websocket"
)

// NdtJSONMessage holds the JSON messages we can receive from the server. We
// only support the subset of the NDT JSON protocol that has two fields: msg,
// and tests.
type NdtJSONMessage struct {
	Msg   string `json:"msg"`
	Tests string `json:"tests,omitempty"`
}

func (n *NdtJSONMessage) String() string {
	b, _ := json.Marshal(n)
	return string(b)
}

// NdtS2CResult is the result object returned to S2C clients as JSON.
type NdtS2CResult struct {
	ThroughputValue  float64
	UnsentDataAmount int64
	TotalSentByte    int64
}

func (n *NdtS2CResult) String() string {
	b, _ := json.Marshal(n)
	return string(b)
}

func readNdtMessage(ws *websocket.Conn, expectedType byte) ([]byte, error) {
	_, inbuff, err := ws.ReadMessage()
	if err != nil {
		return nil, err
	}
	if inbuff[0] != expectedType {
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

func WriteNdtMessage(ws *websocket.Conn, msgType byte, msg fmt.Stringer) error {
	message := msg.String()
	outbuff := make([]byte, 3+len(message))
	outbuff[0] = msgType
	outbuff[1] = byte((len(message) >> 8) & 0xFF)
	outbuff[2] = byte(len(message) & 0xFF)
	for i := range message {
		outbuff[i+3] = message[i]
	}
	return ws.WriteMessage(websocket.BinaryMessage, outbuff)
}

func RecvNdtJSONMessage(ws *websocket.Conn, expectedType byte) (*NdtJSONMessage, error) {
	message := &NdtJSONMessage{}
	jsonString, err := readNdtMessage(ws, expectedType)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(jsonString, &message)
	if err != nil {
		return nil, err
	}
	return message, nil
}

func SendNdtMessage(msgType byte, msg string, ws *websocket.Conn) error {
	message := &NdtJSONMessage{Msg: msg}
	return WriteNdtMessage(ws, msgType, message)
}
