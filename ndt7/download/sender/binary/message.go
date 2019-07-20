// Package binary contains the binary message we send.
package binary

import (
	"math/rand"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

func makeRandomData(size int) []byte {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	data := make([]byte, size)
	// This is not the fastest algorithm to generate a random string, yet it
	// is most likely good enough for our purposes. See [1] for a comprehensive
	// discussion regarding how to generate a random string in Golang.
	//
	// .. [1] https://stackoverflow.com/a/31832326/4354461
	for i := range data {
		data[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return data
}

// Message contains the binary message we send.
type Message struct {
	data []byte
	pm   *websocket.PreparedMessage
	size int
}

// NewMessage creates a new message.
func NewMessage() *Message {
	return &Message{
		data: makeRandomData(spec.MaxMessageSize),
		pm:   nil,
		size: spec.InitialMessageSize,
	}
}

// SetDesiredSize sets the desired message size. The real message size will
// always be included between spec.{Min,Max}MessageSize.
func (m *Message) SetDesiredSize(size int) {
	if size < spec.MinMessageSize {
		m.size = spec.MinMessageSize
	} else if size < spec.MaxMessageSize {
		m.size = size
	} else {
		m.size = spec.MaxMessageSize
	}
	m.pm = nil
}

// Send sends the message over the specified websocket |conn|.
func (m *Message) Send(conn *websocket.Conn) (err error) {
	if m.pm == nil {
		m.pm, err = websocket.NewPreparedMessage(
			websocket.BinaryMessage, m.data[:m.size],
		)
		if err != nil {
			return err
		}
	}
	return conn.WritePreparedMessage(m.pm)
}
