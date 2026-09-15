package measurer

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/go/testingx"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/netx"
)

// netxWSServerConn returns the server side of a websocket accepted through a
// netx.Listener, which is what the measurer needs to read kernel info.
func netxWSServerConn(t *testing.T) *websocket.Conn {
	t.Helper()
	serverConn := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{}
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		serverConn <- c
	}))
	l, err := net.Listen("tcp", ":0")
	testingx.Must(t, err, "listen failed")
	ts.Listener = netx.NewListener(l.(*net.TCPListener))
	ts.Start()
	t.Cleanup(ts.Close)
	url := "ws" + strings.TrimPrefix(ts.URL, "http")
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	testingx.Must(t, err, "dial failed")
	t.Cleanup(func() { c.Close() })
	select {
	case s := <-serverConn:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server conn")
	}
	return nil
}

type fakeAppInfoSource struct{ n int64 }

func (f *fakeAppInfoSource) AppInfo() *model.AppInfo {
	f.n++
	return &model.AppInfo{NumBytes: f.n * 1000, ElapsedTime: f.n}
}

func TestMeasurer_IncludesAppInfoFromSource(t *testing.T) {
	conn := netxWSServerConn(t)
	defer conn.Close()

	src := &fakeAppInfoSource{}
	m := New(conn, "fake-uuid", src)
	ch := m.Start(context.Background(), 400*time.Millisecond)
	var got []model.Measurement
	for meas := range ch {
		got = append(got, meas)
	}
	if len(got) == 0 {
		t.Fatal("no measurements produced")
	}
	for i, meas := range got {
		if meas.AppInfo == nil {
			t.Fatalf("measurement %d has nil AppInfo", i)
		}
		if meas.AppInfo.NumBytes != meas.AppInfo.ElapsedTime*1000 {
			t.Errorf("measurement %d AppInfo = %+v, not from source", i, meas.AppInfo)
		}
		if meas.ConnectionInfo == nil || meas.ConnectionInfo.UUID != "fake-uuid" {
			t.Errorf("measurement %d ConnectionInfo = %+v", i, meas.ConnectionInfo)
		}
	}
}

func TestMeasurer_NilSourceOmitsAppInfo(t *testing.T) {
	conn := netxWSServerConn(t)
	defer conn.Close()

	m := New(conn, "fake-uuid", nil)
	ch := m.Start(context.Background(), 400*time.Millisecond)
	count := 0
	for meas := range ch {
		count++
		if meas.AppInfo != nil {
			t.Errorf("AppInfo = %+v, want nil without a source", meas.AppInfo)
		}
	}
	if count == 0 {
		t.Fatal(fmt.Sprint("no measurements produced"))
	}
}
