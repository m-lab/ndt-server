package ndt7test

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/go/testingx"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

func TestNewNDT7Server(t *testing.T) {
	// Create the ndt7test server.
	h, srv := NewNDT7Server(t)
	defer os.RemoveAll(h.DataDir)

	// Prepare to run a simplified download with ndt7test server.
	URL, _ := url.Parse(srv.URL)
	URL.Scheme = "ws"
	URL.Path = spec.DownloadURLPath
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	headers.Add("User-Agent", "fake-user-agent")
	ctx := context.Background()
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, URL.String(), headers)
	testingx.Must(t, err, "failed to dial websocket ndt7 test")
	err = simpleDownload(ctx, t, conn)
	if err != nil && !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		testingx.Must(t, err, "failed to download")
	}

	// Allow the server time to save the file, the client may stop before the server does.
	time.Sleep(1 * time.Second)
	// Verify a file was saved.
	m, err := filepath.Glob(h.DataDir + "/ndt7/*/*/*/*")
	testingx.Must(t, err, "failed to glob datadir: %s", h.DataDir)
	if len(m) == 0 {
		t.Errorf("no files found")
	}
}

func simpleDownload(ctx context.Context, t *testing.T, conn *websocket.Conn) error {
	defer conn.Close()
	wholectx, cancel := context.WithTimeout(ctx, spec.MaxRuntime)
	defer cancel()
	conn.SetReadLimit(spec.MaxMessageSize)
	err := conn.SetReadDeadline(time.Now().Add(spec.MaxRuntime))
	testingx.Must(t, err, "failed to set read deadline")

	var total int64
	// WARNING: this is not a reference client.
	for wholectx.Err() == nil {
		_, mdata, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		total += int64(len(mdata))
	}
	return nil
}
