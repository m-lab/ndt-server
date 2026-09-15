package receiver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/go/testingx"
	"github.com/m-lab/ndt-server/ndt7/model"
)

// wsPair returns a connected client/server websocket pair.
func wsPair(t *testing.T) (client, server *websocket.Conn) {
	t.Helper()
	serverConn := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		serverConn <- c
	}))
	t.Cleanup(srv.Close)
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	testingx.Must(t, err, "dial failed")
	select {
	case s := <-serverConn:
		return c, s
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server side of websocket pair")
	}
	return nil, nil
}

func TestUploadReceiver_CountsBinaryPayloadOnly(t *testing.T) {
	client, server := wsPair(t)
	defer client.Close()
	defer server.Close()

	data := &model.ArchivalData{}
	recv := StartUploadReceiverAsync(context.Background(), server, data)

	// Before any data, the snapshot is empty.
	if ai := recv.AppInfo(); ai.NumBytes != 0 {
		t.Fatalf("NumBytes before data = %d, want 0", ai.NumBytes)
	}

	sizes := []int{1 << 10, 1 << 13, 1 << 16, 3}
	var want int64
	for _, size := range sizes {
		testingx.Must(t, client.WriteMessage(websocket.BinaryMessage, make([]byte, size)), "write binary")
		want += int64(size)
	}
	// A client measurement message must be archived but not counted.
	testingx.Must(t, client.WriteMessage(websocket.TextMessage,
		[]byte(`{"AppInfo":{"NumBytes":42,"ElapsedTime":1}}`)), "write text")

	deadline := time.Now().Add(5 * time.Second)
	for recv.AppInfo().NumBytes < want && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	ai := recv.AppInfo()
	if ai.NumBytes != want {
		t.Errorf("NumBytes = %d, want %d", ai.NumBytes, want)
	}
	if ai.ElapsedTime <= 0 {
		t.Errorf("ElapsedTime = %d, want > 0", ai.ElapsedTime)
	}

	client.Close()
	<-recv.Done()
	if len(data.ClientMeasurements) != 1 || data.ClientMeasurements[0].AppInfo == nil ||
		data.ClientMeasurements[0].AppInfo.NumBytes != 42 {
		t.Errorf("ClientMeasurements = %+v, want one entry with NumBytes 42", data.ClientMeasurements)
	}
	// Text bytes were not added to the counter.
	if got := recv.AppInfo().NumBytes; got != want {
		t.Errorf("NumBytes after text message = %d, want %d", got, want)
	}
}

func TestUploadReceiver_ElapsedTimeGrows(t *testing.T) {
	client, server := wsPair(t)
	defer client.Close()
	defer server.Close()

	recv := StartUploadReceiverAsync(context.Background(), server, &model.ArchivalData{})
	first := recv.AppInfo().ElapsedTime
	time.Sleep(20 * time.Millisecond)
	second := recv.AppInfo().ElapsedTime
	if second <= first {
		t.Errorf("ElapsedTime did not grow: first=%d second=%d", first, second)
	}
}
