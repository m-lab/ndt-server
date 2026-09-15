// Package handler implements the WebSocket handler for ndt7.
package handler_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/go/testingx"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/ndt7test"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/tcp-info/inetdiag"
)

// fakeServer implements the eventsocket.Server interface for testing the ndt7 handler.
type fakeServer struct {
	created int
	deleted chan bool
}

func (f *fakeServer) Listen() error               { return nil }
func (f *fakeServer) Serve(context.Context) error { return nil }
func (f *fakeServer) FlowCreated(timestamp time.Time, uuid string, sockid inetdiag.SockID) {
	f.created++
}
func (f *fakeServer) FlowDeleted(timestamp time.Time, uuid string) {
	close(f.deleted)
}

func TestHandler_Download(t *testing.T) {
	t.Run("download flow events", func(t *testing.T) {
		fs := &fakeServer{deleted: make(chan bool)}
		ndt7h, srv := ndt7test.NewNDT7Server(t)
		// Override the handler Events server with our fake server.
		ndt7h.Events = fs

		// Run a pseudo test to generate connection events.
		conn, err := simpleConnect(srv.URL)
		testingx.Must(t, err, "failed to dial websocket ndt7 test")
		err = downloadHelper(context.Background(), t, conn)
		if err != nil && !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
			testingx.Must(t, err, "failed to download")
		}
		srv.Close()

		// Verify that both events have occurred once.
		if fs.created == 0 {
			t.Errorf("flow events created not detected; got %d, want 1", fs.created)
		}
		// Since the connection handler goroutine shutdown is independent of the
		// server and client connection shutdowns, wait for the fakeServer to
		// receive the delete flow message up to 15 seconds.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		select {
		case <-ctx.Done():
			t.Errorf("flow events not deleted before timeout")
		case <-fs.deleted:
			// Success.
		}
	})
}

func simpleConnect(srv string) (*websocket.Conn, error) {
	// Prepare to run a simplified download with ndt7test server.
	URL, _ := url.Parse(srv)
	URL.Scheme = "ws"
	URL.Path = spec.DownloadURLPath
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	headers.Add("User-Agent", "fake-user-agent")
	ctx := context.Background()
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, URL.String(), headers)
	return conn, err
}

// WARNING: this is not a reference client.
func downloadHelper(ctx context.Context, t *testing.T, conn *websocket.Conn) error {
	defer conn.Close()
	conn.SetReadLimit(spec.MaxMessageSize)
	err := conn.SetReadDeadline(time.Now().Add(spec.MaxRuntime))
	testingx.Must(t, err, "failed to set read deadline")
	_, _, err = conn.ReadMessage()
	if err != nil {
		return err
	}
	// We only read one message, so this is an early close.
	return conn.Close()
}

func TestHandler_Upload_ServerAppInfo(t *testing.T) {
	_, srv := ndt7test.NewNDT7Server(t)
	defer srv.Close()

	URL, _ := url.Parse(srv.URL)
	URL.Scheme = "ws"
	URL.Path = spec.UploadURLPath
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(context.Background(), URL.String(), headers)
	testingx.Must(t, err, "failed to dial upload")
	defer conn.Close()

	// Read server measurements concurrently until the connection closes.
	msgs := make(chan model.Measurement, 128)
	go func() {
		defer close(msgs)
		conn.SetReadLimit(spec.MaxMessageSize)
		for {
			var m model.Measurement
			if err := conn.ReadJSON(&m); err != nil {
				return
			}
			msgs <- m
		}
	}()

	// Upload for about one second, then close the write side.
	payload := make([]byte, 1<<13)
	var sent int64
	stop := time.Now().Add(1200 * time.Millisecond)
	for time.Now().Before(stop) {
		testingx.Must(t, conn.WriteMessage(websocket.BinaryMessage, payload), "write binary")
		sent += int64(len(payload))
	}
	// Give the measurer a chance to sample after the last bytes landed.
	time.Sleep(700 * time.Millisecond)
	conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	conn.Close()

	var last *model.Measurement
	for m := range msgs {
		if m.AppInfo != nil {
			mm := m
			last = &mm
		}
	}
	if last == nil {
		t.Fatal("no server measurement carried AppInfo during upload")
	}
	if last.AppInfo.NumBytes <= 0 || last.AppInfo.NumBytes > sent {
		t.Errorf("AppInfo.NumBytes = %d, want in (0, %d]", last.AppInfo.NumBytes, sent)
	}
	if last.AppInfo.ElapsedTime <= 0 {
		t.Errorf("AppInfo.ElapsedTime = %d, want > 0", last.AppInfo.ElapsedTime)
	}
	// Application bytes exclude WebSocket framing, so they never exceed the
	// kernel's count of received bytes (TCPInfo is nil off Linux).
	if last.TCPInfo != nil && last.AppInfo.NumBytes > last.TCPInfo.BytesReceived {
		t.Errorf("AppInfo.NumBytes %d > TCPInfo.BytesReceived %d",
			last.AppInfo.NumBytes, last.TCPInfo.BytesReceived)
	}
}
