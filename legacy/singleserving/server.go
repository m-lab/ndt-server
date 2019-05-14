package singleserving

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/m-lab/ndt-server/legacy/ndt"

	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/m-lab/ndt-server/legacy/ws"
	"github.com/m-lab/ndt-server/ndt7/listener"

	"github.com/m-lab/ndt-server/legacy/metrics"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/tcplistener"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	LegacyNDTOpen = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_singleserving_start_total",
			Help: "Number times a single-serving server was started.",
		},
		[]string{"protocol"},
	)
	LegacyNDTClose = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_singleserving_close_total",
			Help: "Number times a single-serving server was closed.",
		},
		[]string{"protocol"},
	)
	LegacyNDTCloseDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "ndt_singleserving_close_duration_seconds",
			Help: "How long did it take to run the Close() method.",
		},
		[]string{"protocol"},
	)
)

// wsServer is a single-serving server for unencrypted websockets.
type wsServer struct {
	srv        *http.Server
	listener   *listener.CachingTCPKeepAliveListener
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

	if err != nil && err != http.ErrServerClosed {
		return nil, fmt.Errorf("Server did not close correctly: %v", err)
	}
	if s.newConn == nil && s.newConnErr == nil {
		return nil, errors.New("No connection created")
	}
	return s.newConn, s.newConnErr
}

func (s *wsServer) Close() {
	s.once.Do(func() {
		LegacyNDTClose.WithLabelValues(string(s.kind)).Inc()
		defer func(start time.Time) {
			LegacyNDTCloseDuration.WithLabelValues(string(s.kind)).Observe(time.Now().Sub(start).Seconds())
		}(time.Now())

		// We need to set the timeout in the future to break the server out of its
		// confusion around the error being temporary. This is a hack.
		s.listener.SetDeadline(time.Now().Add(10 * time.Second))

		// Close the listener first. Accept() on a timed-out channel is a net.Error
		// where .Temporary() returns true. This means that timeouts cause the
		// http.Server.Serve() function to go into an infinite loop waiting for the
		// "temporary" error to be fixed. When the listener is closed and the timeout
		// is still in the future, the error returned is a net.Error where .Temporary()
		// returns false, which terminates the Serve() call.
		s.listener.Close()
		s.srv.Close()
	})
}

// StartWS starts a single-serving unencrypted websocket server. When this
// method returns without error, it is safe for a client to connect to the
// server, as the server socket will be in "listening" mode. The returned
// server will not actually respond until ServeOnce() is called, but the
// connect() will not fail as long as ServeOnce is called soon after this
// returns.
func StartWS(direction string) (ndt.TestServer, error) {
	LegacyNDTOpen.WithLabelValues(string(ndt.WS)).Inc()
	return startWS(direction)
}

func startWS(direction string) (*wsServer, error) {
	mux := http.NewServeMux()
	s := &wsServer{
		srv: &http.Server{
			Handler: mux,
		},
		direction: direction,
		kind:      ndt.WS,
	}
	s.serve = s.srv.Serve
	mux.HandleFunc("/ndt_protocol",
		promhttp.InstrumentHandlerCounter(metrics.TestCount.MustCurryWith(prometheus.Labels{"direction": direction}), s))

	// Start listening right away to ensure that subsequent connections succeed.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}
	tcpl := l.(*net.TCPListener)
	tcpl.SetDeadline(time.Now().Add(10 * time.Second))
	s.listener = &listener.CachingTCPKeepAliveListener{TCPListener: tcpl}
	s.port = s.listener.Addr().(*net.TCPAddr).Port
	return s, nil
}

// wssServer is a single-serving server for encrypted websockets. A wssServer is
// just a wsServer with a different ServeOnce method and two extra fields.
type wssServer struct {
	*wsServer
	certFile, keyFile string
}

// StartWSS starts a single-serving encrypted websocket server. When this method
// returns without error, it is safe for a client to connect to the server, as
// the server socket will be in "listening" mode. Then returned server will not
// actually respond until ServeOnce() is called, but the connect() will not fail
// as long as ServeOnce is called soon after this returns. To prevent the
// accept() call from blocking forever, the server socket has a read deadline
// set 10 seconds in the future. Make sure you call accept() within that window.
func StartWSS(direction, certFile, keyFile string) (ndt.TestServer, error) {
	LegacyNDTOpen.WithLabelValues(string(ndt.WSS)).Inc()
	ws, err := startWS(direction)
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
	listener net.Listener
	port     int
}

func (ps *plainServer) Close() {
	LegacyNDTClose.WithLabelValues(string(ndt.Plain)).Inc()
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
func StartPlain() (ndt.TestServer, error) {
	LegacyNDTOpen.WithLabelValues(string(ndt.Plain)).Inc()
	// Start listening right away to ensure that subsequent connections succeed.
	s := &plainServer{}
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}
	tcpl := l.(*net.TCPListener)
	tcpl.SetDeadline(time.Now().Add(10 * time.Second))
	s.listener = &tcplistener.RawListener{TCPListener: tcpl}
	s.port = s.listener.Addr().(*net.TCPAddr).Port
	return s, nil
}
