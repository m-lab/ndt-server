package protocol_test

import (
	"encoding/json"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/m-lab/go/rtx"
	"github.com/m-lab/ndt-server/ndt5/protocol"
)

func Test_verifyStringConversions(t *testing.T) {
	for m := protocol.MessageType(0); m < 255; m++ {
		if m.String() == "" {
			t.Errorf("MessageType(0x%x) should not result in an empty string", m)
		}
	}
	for _, subtest := range []struct {
		mt  protocol.MessageType
		str string
	}{
		{protocol.SrvQueue, "SrvQueue"},
		{protocol.MsgLogin, "MsgLogin"},
		{protocol.TestPrepare, "TestPrepare"},
		{protocol.TestStart, "TestStart"},
		{protocol.TestMsg, "TestMsg"},
		{protocol.TestFinalize, "TestFinalize"},
		{protocol.MsgError, "MsgError"},
		{protocol.MsgResults, "MsgResults"},
		{protocol.MsgLogout, "MsgLogout"},
		{protocol.MsgWaiting, "MsgWaiting"},
		{protocol.MsgExtendedLogin, "MsgExtendedLogin"},
	} {
		if subtest.mt.String() != subtest.str {
			t.Errorf("%q != %q", subtest.mt.String(), subtest.str)
		}
	}
}

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
		c, err := ln.Accept()
		rtx.Must(err, "Could not accept connection")
		conn := protocol.AdaptNetConn(c, c)
		msg, err := protocol.ReceiveJSONMessage(conn, m.kind)
		rtx.Must(err, "Could not read JSON message")
		if *msg != m.msg {
			t.Errorf("%v != %v", *msg, m.msg)
		}
	}
}

type fakeConnection struct {
	data []byte
	err  error
}

func (fc *fakeConnection) ReadMessage() (int, []byte, error)               { return 0, fc.data, fc.err }
func (fc *fakeConnection) ReadBytes() (count int64, err error)             { return }
func (fc *fakeConnection) WriteMessage(messageType int, data []byte) error { return nil }
func (fc *fakeConnection) FillUntil(t time.Time, buffer []byte) (bytesWritten int64, err error) {
	return
}
func (fc *fakeConnection) ServerIPAndPort() (string, int) { return "", 0 }
func (fc *fakeConnection) ClientIPAndPort() (string, int) { return "", 0 }
func (fc *fakeConnection) Close() error                   { return nil }
func (fc *fakeConnection) UUID() string                   { return "" }
func (fc *fakeConnection) String() string                 { return "" }
func (fc *fakeConnection) Messager() protocol.Messager    { return nil }

func assertFakeConnectionIsConnection(fc *fakeConnection) {
	func(c protocol.Connection) {}(fc)
}

func Test_ReceiveJSONMessage(t *testing.T) {
	type args struct {
		ws           protocol.Connection
		expectedType protocol.MessageType
	}
	tests := []struct {
		name    string
		args    args
		want    *protocol.JSONMessage
		wantErr bool
	}{
		{
			args: args{
				ws: &fakeConnection{
					data: nil,
					err:  nil,
				},
				expectedType: protocol.TestMsg,
			},
			wantErr: true,
		},
		{
			args: args{
				ws: &fakeConnection{
					data: append([]byte{byte(protocol.TestMsg), 0, 14}, []byte(`{"msg": "125"}`)...),
					err:  nil,
				},
				expectedType: protocol.TestMsg,
			},
			want: &protocol.JSONMessage{
				Msg: "125",
			},
		},
		{
			args: args{
				ws: &fakeConnection{
					data: append([]byte{byte(protocol.TestMsg), 0, 3}, []byte(`125`)...),
					err:  nil,
				},
				expectedType: protocol.TestMsg,
			},
			want: &protocol.JSONMessage{
				Msg: "125",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := protocol.ReceiveJSONMessage(tt.args.ws, tt.args.expectedType)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReceiveJSONMessage() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReceiveJSONMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}
