package singleserving

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	ndt5metrics "github.com/m-lab/ndt-server/ndt5/metrics"
	"github.com/m-lab/ndt-server/ndt5/ndt"
	"github.com/m-lab/ndt-server/ndt5/protocol"
	"github.com/m-lab/ndt-server/ndt5/ws"
	"github.com/m-lab/ndt-server/netx"
)

// wsServer is a single-serving server for unencrypted websockets.
type wsServer struct {
	srv        *http.Server
	listener   *netx.Listener
	port       int
	direction  string
	newConn    protocol.MeasuredConnection
	newConnErr error
	once       sync.Once
	kind       ndt.ConnectionType
	serve      func(net.Listener) error
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
	go s.Close()
}

func (s *wsServer) Port() int {
	return s.port
}

func (s *wsServer) ServeOnce(ctx context.Context) (protocol.MeasuredConnection, error) {
	// This is a single-serving server. After serving one response, shut it down.
	defer s.Close()

	derivedCtx, derivedCancel := context.WithCancel(ctx)
	var closeErr error
	go func() {
		closeErr = s.serve(s.listener)
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
	if s.newConn == nil && err != nil && err != http.ErrServerClosed {
		log.Println("Server closed incorrectly:", err)
		return nil, errors.New("Server did not close correctly")
	}

	// If the context times out, then we can arrive here with both the connection
	// and error being nil.
	if s.newConn == nil && s.newConnErr == nil {
		return nil, errors.New("No connection created")
	}
	// Because the client has contacted the test server successfully, count the test.
	ndt5metrics.MeasurementServerAccept.WithLabelValues(s.kind.String(), s.direction)
	return s.newConn, s.newConnErr
}

func (s *wsServer) Close() {
	s.once.Do(func() {
		ndt5metrics.MeasurementServerStop.WithLabelValues(string(s.kind)).Inc()
		s.listener.Close()
		s.srv.Close()
	})
}

// ListenWS starts a single-serving unencrypted websocket server. When this
// method returns without error, it is safe for a client to connect to the
// server, as the server socket will be in "listening" mode. The returned server
// will not actually respond until ServeOnce() is called, but the connect() will
// not fail as long as ServeOnce is called soon ("soon" is defined by os-level
// timeouts) after this returns.
func ListenWS(direction string) (ndt.SingleMeasurementServer, error) {
	ndt5metrics.MeasurementServerStart.WithLabelValues(string(ndt.WS)).Inc()
	return listenWS(direction)
}

func listenWS(direction string) (*wsServer, error) {
	mux := http.NewServeMux()
	s := &wsServer{
		srv: &http.Server{
			Handler: mux,
			// NOTE: set absolute read and write timeouts for server connections.
			// This prevents clients, or middleboxes, from opening a connection and
			// holding it open indefinitely. This applies equally to TLS and non-TLS
			// servers.
			ReadTimeout:  time.Minute,
			WriteTimeout: time.Minute,
		},
		direction: direction,
		kind:      ndt.WS,
	}
	s.serve = s.srv.Serve
	mux.Handle("/ndt_protocol", s)

	// Start listening right away to ensure that subsequent connections succeed.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}
	tcpl := l.(*net.TCPListener)
	s.port = tcpl.Addr().(*net.TCPAddr).Port
	s.listener = netx.NewListener(tcpl)
	return s, nil
}

// wssServer is a single-serving server for encrypted websockets. A wssServer is
// just a wsServer with a different start method and two extra fields.
type wssServer struct {
	*wsServer
	certFile, keyFile string
}

// ListenWSS starts a single-serving encrypted websocket server. When this method
// returns without error, it is safe for a client to connect to the server, as
// the server socket will be in "listening" mode. The returned server will not
// actually respond until ServeOnce() is called, but the connect() will not fail
// as long as ServeOnce is called soon ("soon" is defined by os-level timeouts)
// after this returns.
func ListenWSS(direction, certFile, keyFile string) (ndt.SingleMeasurementServer, error) {
	ndt5metrics.MeasurementServerStart.WithLabelValues(string(ndt.WSS)).Inc()
	ws, err := listenWS(direction)
	if err != nil {
		return nil, err
	}
	wss := wssServer{
		wsServer: ws,
		certFile: certFile,
		keyFile:  keyFile,
	}
	wss.kind = ndt.WSS
	wss.serve = func(l net.Listener) error {
		return wss.srv.ServeTLS(l, wss.certFile, wss.keyFile)
	}
	return &wss, nil
}

// plainServer is a single-serving server for plain TCP sockets.
type plainServer struct {
	listener  net.Listener
	port      int
	direction string
}

func (ps *plainServer) Close() {
	ndt5metrics.MeasurementServerStop.WithLabelValues(string(ndt.Plain)).Inc()
	ps.listener.Close()
}

func (ps *plainServer) Port() int {
	return ps.port
}

func (ps *plainServer) ServeOnce(ctx context.Context) (protocol.MeasuredConnection, error) {
	// NOTE: set an absolute timeouts for single serving servers.
	derivedCtx, derivedCancel := context.WithTimeout(ctx, time.Minute)
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
	if conn == nil {
		return nil, errors.New("nil conn, nil err: " + derivedCtx.Err().Error())
	}
	// Because the client has contacted the test server successfully, count the test.
	ndt5metrics.MeasurementServerAccept.WithLabelValues(ndt.Plain.String(), ps.direction)
	return protocol.AdaptNetConn(conn, conn), nil
}

// ListenPlain starts a single-serving server for plain NDT tests. When this
// method returns without error, it is safe for a client to connect to the
// server, as the server socket will be in "listening" mode. The returned server
// will not actually respond until ServeOnce() is called, but the connect() will
// not fail as long as ServeOnce is called soon ("soon" is defined by os-level
// timeouts) after this returns.
func ListenPlain(direction string) (ndt.SingleMeasurementServer, error) {
	ndt5metrics.MeasurementServerStart.WithLabelValues(string(ndt.Plain)).Inc()
	// Start listening right away to ensure that subsequent connections succeed.
	s := &plainServer{
		direction: direction,
	}
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}
	tcpl := l.(*net.TCPListener)
	s.port = tcpl.Addr().(*net.TCPAddr).Port
	s.listener = netx.NewListener(tcpl)
	return s, nil
}
