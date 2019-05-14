package singleserving

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/m-lab/ndt-server/legacy/ws"
	"github.com/m-lab/ndt-server/ndt7/listener"

	"github.com/m-lab/ndt-server/legacy/metrics"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/tcplistener"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Factory is the method by which we abstract away what kind of server is being
// created at any given time. Using this abstraction allows us to use almost the
// same code for WS and WSS.
type Factory interface {
	SingleServingServer(direction string) (Server, error)
}

// Server is the interface implemented by every single-serving server. No matter
// whether they use WSS, WS, TCP with JSON, or TCP without JSON.
type Server interface {
	Port() int
	ServeOnce(context.Context) (protocol.MeasuredConnection, error)
	Close()
}

// wsServer is a single-serving server for unencrypted websockets.
type wsServer struct {
	srv        *http.Server
	listener   net.Listener
	port       int
	direction  string
	newConn    protocol.MeasuredConnection
	newConnErr error
}

func (s *wsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upgrader := ws.Upgrader(s.direction)
	wsc, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.newConnErr = err
		return
	}
	s.newConn = protocol.AdaptWsConn(wsc)
	// The websocket upgrade process hijacks the connection. Only un-hijacked
	// connections are terminated on server shutdown.
	s.Close()
}

func (s *wsServer) Port() int {
	return s.port
}

func (s *wsServer) ServeOnce(ctx context.Context) (protocol.MeasuredConnection, error) {
	// This is a single-serving server. After serving one response, shut it down.
	defer func() {
		go s.Close()
	}()

	derivedCtx, derivedCancel := context.WithCancel(ctx)
	var closeErr error
	go func() {
		closeErr = s.srv.Serve(s.listener)
		derivedCancel()
	}()
	// This will wait until derivedCancel() is called or the parent context is
	// canceled. It is the parent context that determines how long ServeOnce should
	// wait before giving up.
	<-derivedCtx.Done()

	// During error conditions there is a race with the goroutine to write to
	// closeErr. We copy the current value to a separate variable in an effort to
	// ensure that the race gets resolved in just one way for the following if().
	err := closeErr

	if err != nil && err != http.ErrServerClosed {
		return nil, fmt.Errorf("Server did not close correctly: %v", err)
	}
	return s.newConn, s.newConnErr
}

func (s *wsServer) Close() {
	s.srv.Close()
	s.listener.Close()
}

// StartWS starts a single-serving unencrypted websocket server. When this
// method returns without error, it is safe for a client to connect to the
// server, as the server socket will be in "listening" mode. The returned
// server will not actually respond until ServeOnce() is called, but the
// connect() will not fail as long as ServeOnce is called soon after this
// returns.
func StartWS(direction string) (Server, error) {
	mux := http.NewServeMux()
	s := &wsServer{
		srv: &http.Server{
			Handler: mux,
		},
		direction: direction,
	}
	mux.HandleFunc("/ndt_protocol",
		promhttp.InstrumentHandlerCounter(metrics.TestCount.MustCurryWith(prometheus.Labels{"direction": direction}), s))

	// Start listening right away to ensure that subsequent connections succeed.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}
	tcpl := l.(*net.TCPListener)
	tcpl.SetDeadline(time.Now().Add(10 * time.Second))
	s.listener = listener.CachingTCPKeepAliveListener{TCPListener: tcpl}
	s.port = s.listener.Addr().(*net.TCPAddr).Port
	return s, nil
}

// wssServer is a single-serving server for encrypted websockets. A wssServer is
// just a wsServer with a different ServeOnce method and two extra fields.
type wssServer struct {
	*wsServer
	certFile, keyFile string
}

func (wss *wssServer) ServeOnce(ctx context.Context) (protocol.MeasuredConnection, error) {
	err := wss.srv.ServeTLS(wss.listener, wss.certFile, wss.keyFile)
	if err != http.ErrServerClosed {
		return nil, err
	}
	return wss.newConn, wss.newConnErr
}

// StartWSS starts a single-serving encrypted websocket server. When this method
// returns without error, it is safe for a client to connect to the server, as
// the server socket will be in "listening" mode. Then returned server will not
// actually respond until ServeOnce() is called, but the connect() will not fail
// as long as ServeOnce is called soon after this returns. To prevent the
// accept() call from blocking forever, the server socket has a read deadline
// set 10 seconds in the future. Make sure you call accept() within that window.
func StartWSS(direction, certFile, keyFile string) (Server, error) {
	ws, err := StartWS(direction)
	if err != nil {
		return nil, err
	}
	wss := wssServer{
		wsServer: ws.(*wsServer),
		certFile: certFile,
		keyFile:  keyFile,
	}
	return &wss, nil
}

// plainServer is a single-serving server for plain TCP sockets.
type plainServer struct {
	listener net.Listener
	port     int
}

func (ps *plainServer) Close() {
	ps.listener.Close()
}

func (ps *plainServer) Port() int {
	return ps.port
}

func (ps *plainServer) ServeOnce(ctx context.Context) (protocol.MeasuredConnection, error) {
	derivedCtx, derivedCancel := context.WithCancel(ctx)
	defer ps.Close()

	var conn net.Conn
	var err error
	go func() {
		conn, err = ps.listener.Accept()
		derivedCancel()
	}()
	<-derivedCtx.Done()

	if err != nil {
		return nil, err
	}
	return protocol.AdaptNetConn(conn, conn), nil
}

// StartPlain starts a single-serving server for plain NDT tests. To prevent the
// accept() call from blocking forever, the server socket has a read deadline
// set 10 seconds in the future. Make sure you call accept() within that window.
func StartPlain() (Server, error) {
	// Start listening right away to ensure that subsequent connections succeed.
	s := &plainServer{}
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}
	tcpl := l.(*net.TCPListener)
	tcpl.SetDeadline(time.Now().Add(10 * time.Second))
	s.listener = tcplistener.RawListener{TCPListener: tcpl}
	s.port = s.listener.Addr().(*net.TCPAddr).Port
	return s, nil
}
