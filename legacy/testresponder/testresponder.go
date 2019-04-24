package testresponder

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/tcplistener"
)

// ServerType indicates what type of NDT test the particular server is
// performing. There are extant active clients for each of these protocol
// variations.
type ServerType int

const (
	// RawNoJSON NDT tests correspond to the code integrated into the uTorrent client.
	RawNoJSON = ServerType(iota)
	// RawJSON NDT tests correspond to code that integrated web100clt anytime after 2016.
	RawJSON
	// WS NDT tests take place over unencrypted websockets.
	WS
	// WSS NDT tests take place over encrypted websockets.
	WSS
)

// Config expresses the configuration of the server, and whether to use TLS or not.
type Config struct {
	KeyFile, CertFile string
	ServerType        ServerType
}

// TestResponder coordinates synchronization between the main control loop and subtests.
type TestResponder struct {
	Port   int
	Ln     net.Listener
	S      *http.Server
	Ctx    context.Context
	Cancel context.CancelFunc
	Config *Config
}

// MakeNdtUpgrader creates a websocket Upgrade for the NDT legacy
func MakeNdtUpgrader(protocols []string) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:    81920,
		WriteBufferSize:   81920,
		Subprotocols:      protocols,
		EnableCompression: false,
		CheckOrigin: func(r *http.Request) bool {
			// TODO: make this check more appropriate -- added to get initial html5 widget to work.
			return true
		},
	}
}

// Listen on a random port.
func listenRandom() (net.Listener, int, error) {
	// Start listening
	ln, err := net.ListenTCP("tcp", &net.TCPAddr{})
	if err != nil {
		return nil, 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	return tcplistener.RawListener{TCPListener: ln}, port, nil
}

// StartAsync allocates a new TLS HTTP server listening on a random port. The
// server can be stopped again using TestResponder.Close().
func (tr *TestResponder) StartAsync(ctx context.Context, mux *http.ServeMux, rawTest func(protocol.MeasuredConnection), msg string) error {
	tr.Ctx, tr.Cancel = context.WithCancel(ctx)
	ln, port, err := listenRandom()
	if err != nil {
		log.Println("ERROR: Failed to listen on any port:", err)
		return err
	}
	tr.Port = port
	tr.Ln = ln
	tr.S = &http.Server{Handler: mux}
	go func() {
		log.Printf("%s: Serving for test on %s", msg, ln.Addr())
		var err error
		switch tr.Config.ServerType {
		case WSS:
			err = tr.S.ServeTLS(ln, tr.Config.CertFile, tr.Config.KeyFile)
		case WS:
			err = tr.S.Serve(ln)
		case RawJSON:
			err = tr.serveRaw(ln, rawTest)
		default:
			err = errors.New("Can't start server")
		}
		if err != nil && err != http.ErrServerClosed {
			log.Printf("ERROR: %s Starting server: %s", msg, err)
		}
	}()
	return nil
}

func (tr *TestResponder) serveRaw(ln net.Listener, fn func(protocol.MeasuredConnection)) error {
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	fn(protocol.AdaptNetConn(conn, conn))
	return nil
}

// Close will shutdown, cancel, or close all resources used by the test.
func (tr *TestResponder) Close() {
	log.Println("Closing Test Responder")
	if tr.S != nil {
		// Shutdown the server for the test.
		tr.S.Close()
		tr.S = nil
	}
	if tr.Ln != nil {
		// Shutdown the socket listener.
		tr.Ln.Close()
		tr.Ln = nil
	}
	if tr.Cancel != nil {
		// Cancel the test responder context.
		tr.Cancel()
		tr.Cancel = nil
	}
}
