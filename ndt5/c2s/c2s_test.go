package c2s

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/m-lab/go/rtx"
	"github.com/m-lab/ndt-server/ndt5/protocol"
	"github.com/m-lab/ndt-server/ndt5/singleserving"
	"github.com/m-lab/ndt-server/netx"
)

func MustMakeNetConnection(ctx context.Context) (protocol.MeasuredConnection, net.Conn) {
	tcpl, err := net.Listen("tcp", "127.0.0.1:0")
	rtx.Must(err, "Could not listen")
	tl := netx.NewListener(tcpl.(*net.TCPListener))
	conns := make(chan net.Conn)
	defer close(conns)
	go func() {
		clientConn, err := net.Dial("tcp", tcpl.Addr().String())
		rtx.Must(err, "Could not dial temp conn")
		conns <- clientConn
	}()
	conn, err := tl.Accept()
	rtx.Must(err, "Could not accept")
	return protocol.AdaptNetConn(conn, conn), <-conns
}

func Test_DrainForeverButMeasureFor_NormalOperation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sConn, cConn := MustMakeNetConnection(ctx)
	defer sConn.Close()
	defer cConn.Close()
	// Send for longer than we measure.
	go func() {
		ctx2, cancel2 := context.WithTimeout(ctx, 10*time.Second)
		defer cancel2() // Useless, but makes the linter happpy.
		for ctx2.Err() == nil {
			cConn.Write([]byte("hello"))
		}
		cConn.Close()
	}()
	metrics, err := drainForeverButMeasureFor(ctx, sConn, time.Duration(500*time.Millisecond))
	if err != nil {
		t.Fatal("Should not have gotten error:", err)
	}
	if metrics.TCPInfo.BytesReceived <= 0 {
		t.Errorf("Expected positive byte count but got %d", metrics.TCPInfo.BytesReceived)
	}
}

func Test_DrainForeverButMeasureFor_EarlyClientQuit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sConn, cConn := MustMakeNetConnection(ctx)
	defer sConn.Close()
	defer cConn.Close()
	// Measure longer than we send.
	go func() {
		for i := 0; i < 10; i++ {
			cConn.Write([]byte("hello"))
		}
		time.Sleep(150 * time.Millisecond) // Give the drainForever process time to get going
		cConn.Close()
	}()
	metrics, err := drainForeverButMeasureFor(ctx, sConn, time.Duration(4*time.Second))
	if err == nil {
		t.Fatal("Should have gotten an error")
	}
	if metrics.TCPInfo.BytesReceived <= 0 {
		t.Errorf("Expected positive byte count but got %d", metrics.TCPInfo.BytesReceived)
	}
}

func MustMakeWsConnection(ctx context.Context) (protocol.MeasuredConnection, *websocket.Conn) {
	srv, err := singleserving.ListenWS("c2s")
	rtx.Must(err, "Could not listen")
	conns := make(chan *websocket.Conn)
	defer close(conns)
	go func() {
		d := websocket.Dialer{}
		// This will actually result in a failed websocket connection attempt because
		// we aren't setting any headers. That's okay for testing purposes, as we are
		// trying to make sure that the underlying socket stats are counted, and the
		// failed upgrade will still result in non-zero socket stats.
		clientConn, _, err := d.Dial("ws://localhost:"+strconv.Itoa(srv.Port())+"/ndt_protocol", http.Header{})
		rtx.Must(err, "Could not dial temp conn")
		conns <- clientConn
	}()
	conn, err := srv.ServeOnce(ctx)
	rtx.Must(err, "Could not accept")
	return conn, <-conns
}

func Test_DrainForeverButMeasureFor_CountsAllBytesNotJustWsGoodput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sConn, cConn := MustMakeWsConnection(ctx)
	defer sConn.Close()
	defer cConn.Close()
	// Send for longer than we measure.
	go func() {
		// Send nothing. But the websocket handshake used some bytes, so the underlying socket should not measure zero.
		ctx2, cancel2 := context.WithTimeout(ctx, 10*time.Second)
		defer cancel2() // Useless, but makes the linter happpy.
		<-ctx2.Done()
		cConn.Close()
	}()
	metrics, err := drainForeverButMeasureFor(ctx, sConn, time.Duration(100*time.Millisecond))
	if err != nil {
		t.Fatal("Should not have gotten error:", err)
	}
	if metrics.TCPInfo.BytesReceived <= 0 {
		t.Errorf("Expected positive byte count but got %d", metrics.TCPInfo.BytesReceived)
	}
}
