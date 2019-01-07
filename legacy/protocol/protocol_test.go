package protocol_test

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/m-lab/go/rtx"
	"github.com/m-lab/ndt-cloud/legacy/protocol"
)

func Test_netConnReadJSONMessage(t *testing.T) {
	// Set up a listener
	ln, err := net.Listen("tcp", "")
	rtx.Must(err, "Could not start test listener")
	type Test struct {
		kind protocol.MessageType
		msg  protocol.JSONMessage
	}

	for _, m := range []Test{
		{kind: protocol.MsgLogin, msg: protocol.JSONMessage{Tests: "22"}},
	} {
		// In a goroutine, create a client and send the listener a message
		go func(m Test) {
			conn, err := net.Dial("tcp", ln.Addr().String())
			rtx.Must(err, "Could not connect to local server")
			bytes, err := json.Marshal(m.msg)
			firstThree := []byte{byte(m.kind), byte(len(bytes) >> 8), byte(len(bytes))}
			_, err = conn.Write(append(firstThree, bytes...))
			rtx.Must(err, "Could not perform write")
		}(m)

		// Ensure that the message was received and parsed properly.
		conn, err := ln.Accept()
		rtx.Must(err, "Could not accept connection")
		msg, err := protocol.ReceiveJSONMessage(protocol.AdaptNetConn(conn, conn), m.kind)
		rtx.Must(err, "Could not read JSON message")
		if *msg != m.msg {
			t.Errorf("%v != %v", *msg, m.msg)
		}
	}
}
