package singleserving

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/legacy/metrics"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

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
	upgrader := websocket.Upgrader{
		ReadBufferSize:    81920,
		WriteBufferSize:   81920,
		Subprotocols:      []string{s.direction},
		EnableCompression: false,
		CheckOrigin: func(r *http.Request) bool {
			return true // Always pass the check.
		},
	}
	wsc, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.newConnErr = err
		return
	}
	s.newConn = protocol.AdaptWsConn(wsc)
	s.Close()
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

	if err != http.ErrServerClosed {
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
// server, as the server socket will be in "listening" mode. Then returned
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
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}
	s.listener = listener
	address := listener.Addr().String()
	s.srv.Addr = address
	chunks := strings.Split(address, ":")
	portStr := chunks[len(chunks)-1]
	s.port, err = strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// wssServer is a single-serving server for encrypted websockets. A wssServer is
// just a wsServer with a different ServeOnce method.
type wssServer struct {
	ws                *wsServer
	certFile, keyFile string
}

func (wss *wssServer) ServeOnce(ctx context.Context) (protocol.MeasuredConnection, error) {
	err := wss.ws.srv.ServeTLS(wss.ws.listener, wss.certFile, wss.keyFile)
	if err != http.ErrServerClosed {
		return nil, err
	}
	return wss.ws.newConn, wss.ws.newConnErr
}

func (wss *wssServer) Port() int {
	return wss.ws.Port()
}

func (wss *wssServer) Close() {
	wss.ws.Close()
}

// StartWSS starts a single-serving encrypted websocket server. When this method
// returns without error, it is safe for a client to connect to the server, as
// the server socket will be in "listening" mode. Then returned server will not
// actually respond until ServeOnce() is called, but the connect() will not fail
// as long as ServeOnce is called soon after this returns.
func StartWSS(direction, certFile, keyFile string) (Server, error) {
	ws, err := StartWS(direction)
	if err != nil {
		return nil, err
	}
	wss := wssServer{
		ws:       ws.(*wsServer),
		certFile: certFile,
		keyFile:  keyFile,
	}
	return &wss, nil
}
