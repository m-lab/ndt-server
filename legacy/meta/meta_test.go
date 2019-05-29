package meta

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/m-lab/ndt-server/legacy/protocol"
)

type sendMessage struct {
	t   protocol.MessageType
	msg []byte
}
type recvMessage struct {
	msg []byte
	err error
}
type fakeMessager struct {
	sent []sendMessage
	recv []recvMessage
	c    int
}

func (m *fakeMessager) SendMessage(t protocol.MessageType, msg []byte) error {
	m.sent = append(m.sent, sendMessage{t: t, msg: msg})
	return nil
}
func (m *fakeMessager) ReceiveMessage(t protocol.MessageType) ([]byte, error) {
	if len(m.recv) <= m.c {
		return []byte(""), nil
	}
	msg, err := m.recv[m.c].msg, m.recv[m.c].err
	m.c++
	if err != nil {
		return nil, err
	}
	return msg, nil
}
func (m *fakeMessager) SendS2CResults(throughputKbps, unsentBytes, totalSentBytes int64) error {
	// Unused.
	return nil
}
func (m *fakeMessager) Encoding() protocol.Encoding {
	// Unused.
	return protocol.JSON
}

var len32 = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ012345")
var len64 = append(len32, len32...)
var len128 = append(len64, len64...)
var len256 = append(len128, len128...)

func TestManageTest(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		m       protocol.Messager
		want    ArchivalData
		wantErr bool
	}{
		{
			name: "success",
			ctx:  context.Background(),
			m: &fakeMessager{
				recv: []recvMessage{
					{
						msg: []byte("a:b"),
					},
				},
			},
			want: []NameValue{
				{Name: "a", Value: "b"},
			},
		},
		{
			name: "truncate-name-to-63-bytes",
			ctx:  context.Background(),
			m: &fakeMessager{
				recv: []recvMessage{
					{
						msg: append(len64, []byte(":b")...),
					},
				},
			},
			want: []NameValue{
				{Name: string(len64[:63]), Value: "b"},
			},
		},
		{
			name: "truncate-value-to-255-bytes",
			ctx:  context.Background(),
			m: &fakeMessager{
				recv: []recvMessage{
					{
						msg: append([]byte("a:"), len256...),
					},
				},
			},
			want: []NameValue{
				{Name: "a", Value: string(len256[:255])},
			},
		},
		{
			name: "receive-error",
			ctx:  context.Background(),
			m: &fakeMessager{
				recv: []recvMessage{
					{
						err: fmt.Errorf("Fake failure to ReceiveMessage"),
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ManageTest(tt.ctx, tt.m)
			if (err != nil) != tt.wantErr {
				t.Errorf("ManageTest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ManageTest() = %v, want %v", got, tt.want)
			}
		})
	}
}
