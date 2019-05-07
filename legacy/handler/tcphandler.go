package handler

import (
	"bufio"
	"context"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/legacy"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/singleserving"
	"github.com/m-lab/ndt-server/metrics"
)

// rawServer handles requests that are TCP-based but not HTTP(S) based. If it
// receives an HTTP test it will forward that test to the ForwardingAddress.
type rawServer struct {
	wsAddr   string
	dialer   *net.Dialer
	listener *net.TCPListener
}

func (rh *rawServer) SingleServingServer(direction string) (singleserving.Server, error) {
	// TODO: create one for raw connections
	return nil, nil
}

// sniffThenHandle implements protocol sniffing to allow WS clients and just-TCP
// clients to connect to the same port. This was a mistake to implement the
// first time, but enough clients exist that need it that we are keeping it in
// this code. In the future, if you are thinking of adding protocol sniffing to
// your system, don't.
func (rs *rawServer) sniffThenHandle(conn net.Conn) {
	// Set up our observation of the duration of this function.
	connectionType := "tcp" // This value may change. Don't close over its value until after the sniffing procedure.
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
		fwd, err := rs.dialer.Dial("tcp", rs.wsAddr)
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
	defer warnonerror.Close(conn, "Could not close connection")

	// First, send the kickoff message (which is only sent for non-WS clients),
	// then transition to the protocol engine where everything should be the same
	// for TCP, WS, and WSS.
	kickoff := "123456 654321"
	n, err := conn.Write([]byte(kickoff))
	if n != len(kickoff) || err != nil {
		log.Printf("Could not write %d byte kickoff string: %d bytes written err: %v\n", len(kickoff), n, err)
	}
	legacy.HandleControlChannel(protocol.AdaptNetConn(conn, input), rs)
}

// ListenAndServe starts up the sniffing server that delegates to the
// appropriate just-TCP or WS protocol.Connection.
func (rs *rawServer) ListenAndServe(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	rs.listener = ln.(*net.TCPListener)
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
				if ctx.Err() != nil {
					break
				}
				log.Println("Failed to accept connection:", err)
				continue
			}
			go func() {
				defer func() {
					r := recover()
					if r != nil {
						log.Println("Recovered from panic in RawServer", r)
					}
				}()
				rs.sniffThenHandle(conn)
			}()
		}
	}()
	return nil
}

func (rs *rawServer) Addr() net.Addr {
	return rs.listener.Addr()
}

// RawForwardingServer is the interface implemented by the non-HTTP-based NDT server.
// Because it isn't run by the http.Server machinery, it has its own interface.
type RawForwardingServer interface {
	ListenAndServe(ctx context.Context, addr string) error
	Addr() net.Addr
}

// NewTCP creates a new TCP listener to serve the client. It forwards all
// connection requests that look like HTTP to a different address (assumed to be
// on the same host).
func NewTCP(wsAddr string) RawForwardingServer {
	return &rawServer{
		wsAddr: wsAddr,
		// The dialer is only contacting localhost. The timeout should be set to a
		// small number.
		dialer: &net.Dialer{
			Timeout: 10 * time.Millisecond,
		},
	}
}
