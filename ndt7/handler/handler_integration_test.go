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
