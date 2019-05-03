package handler

import (
	"bufio"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/m-lab/ndt-server/legacy"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/singleserving"
	"github.com/m-lab/ndt-server/legacy/ws"
	"github.com/m-lab/ndt-server/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type httpFactory struct{}

func (hf *httpFactory) SingleServingServer(dir string) (singleserving.Server, error) {
	return singleserving.StartWS(dir)
}

// httpHandler handles requests that come in over HTTP or HTTPS. It should be
// created with MakeHTTPHandler() or MakeHTTPSHandler().
type httpHandler struct {
	labels        prometheus.Labels
	serverFactory singleserving.Factory
}

// ServeHTTP is the command channel for the NDT-WS or NDT-WSS test. All
// subsequent client communication is synchronized with this method. Returning
// closes the websocket connection, so only occurs after all tests complete or
// an unrecoverable error. It is called ServeHTTP to make sure that the Server
// implements the http.Handler interface.
func (s *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upgrader := ws.Upgrader("ndt")
	wsc, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ERROR SERVER:", err)
		return
	}
	ws := protocol.AdaptWsConn(wsc)
	defer ws.Close()
	legacy.HandleControlChannel(ws, s.serverFactory)
}

// NewWS returns a handler suitable for http-based connections.
func NewWS() http.Handler {
	return &httpHandler{
		serverFactory: &httpFactory{},
	}
}

type httpsFactory struct {
	certFile string
	keyFile  string
}

func (hf *httpsFactory) SingleServingServer(dir string) (singleserving.Server, error) {
	return singleserving.StartWSS(dir, hf.certFile, hf.keyFile)
}

// NewWSS returns a handler suitable for https-based connections.
func NewWSS(certFile, keyFile string) http.Handler {
	return &httpHandler{
		serverFactory: &httpsFactory{
			certFile: certFile,
			keyFile:  keyFile,
		},
	}
}

// rawHandler handles requests that are TCP-based but not HTTP(S) based. If it
// receives an HTTP test it will forward that test to the ForwardingAddress.
type rawHandler struct {
	ForwardingAddr string
	Labels         prometheus.Labels
}

func (rh *rawHandler) SingleServingServer(direction string) (singleserving.Server, error) {
	// TODO: create one for raw connections
	return nil, nil
}

// sniffThenHandle implements protocol sniffing to allow WS clients and just-TCP
// clients to connect to the same port. This was a mistake to implement the
// first time, but enough clients exist that need it that we are keeping it in
// this code. In the future, if you are thinking of adding protocol sniffing to
// your system, don't.
func (rh *rawHandler) sniffThenHandle(conn net.Conn) {
	// Set up our observation of the duration of this function.
	connectionType := "tcp" // This type may change. Don't close over its value until after the sniffing procedure.
	startTime := time.Now()
	defer func() {
		endTime := time.Now()
		metrics.TestDuration.WithLabelValues("legacy", connectionType).Observe(endTime.Sub(startTime).Seconds())
	}()
	// Peek at the first three bytes. If they are "GET", then this is an HTTP
	// conversation and should be forwarded to the HTTP server.
	input := bufio.NewReader(conn)
	lead, err := input.Peek(3)
	if err != nil {
		log.Println("Could not handle connection", conn, "due to", err)
		return
	}
	if string(lead) == "GET" {
		// Forward HTTP-like handshakes to the HTTP server. Note that this does NOT
		// introduce overhead for the s2c and c2s tests, because in those tests the
		// HTTP server itself opens the testing port, and that server will not use
		// this TCP proxy.
		//
		// We must forward instead of doing an HTTP redirect because existing deployed
		// clients don't support redirects, e.g.
		//    https://github.com/websockets/ws/issues/812
		connectionType = "forwarded_ws"
		fwd, err := net.Dial("tcp", rh.ForwardingAddr)
		if err != nil {
			log.Println("Could not forward connection", err)
			return
		}
		wg := sync.WaitGroup{}
		wg.Add(2)
		// Copy the input channel.
		go func() {
			io.Copy(fwd, input)
			wg.Done()
		}()
		// Copy the ouput channel.
		go func() {
			io.Copy(conn, fwd)
			wg.Done()
		}()
		// When both Copy calls are done, close everything.
		go func() {
			wg.Wait()
			conn.Close()
			fwd.Close()
		}()
		return
	}
	// If there was no error and there was no GET, then this should be treated as a
	// legitimate attempt to perform a non-ws-based NDT test.

	// First, send the kickoff message (which is only sent for non-WS clients),
	// then transition to the protocol engine where everything should be the same
	// for TCP, WS, and WSS.
	kickoff := "123456 654321"
	n, err := conn.Write([]byte(kickoff))
	if n != len(kickoff) || err != nil {
		log.Printf("Could not write %d byte kickoff string: %d bytes written err: %v\n", len(kickoff), n, err)
	}
	legacy.HandleControlChannel(protocol.AdaptNetConn(conn, input), rh)
}

// ListenAndServeRawAsync starts up the sniffing server that delegates to the
// appropriate just-TCP or WS protocol.Connection.
func (rh *rawHandler) ListenAndServeRawAsync(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	// Close the listener when the context is canceled. We do this in a separate
	// goroutine to ensure that context cancellation interrupts the Accept() call.
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	// Serve requests until the context is canceled.
	go func() {
		for ctx.Err() == nil {
			conn, err := ln.Accept()
			if err != nil {
				log.Println("Failed to accept connection:", err)
				continue
			}
			go func() {
				rh.sniffThenHandle(conn)
			}()
		}
	}()
	return nil
}

// RawHandler is the interface implemented by the non-HTTP-based NDT server.
// Because it isn't run by the http.Server machinery, it has its own interface.
type RawHandler interface {
	ListenAndServeRawAsync(ctx context.Context, addr string) error
}

func NewTCP(forwardingaddr string) RawHandler {
	return &rawHandler{
		ForwardingAddr: forwardingaddr,
	}
}
