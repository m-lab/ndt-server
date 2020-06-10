package plain

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/m-lab/ndt-server/ndt5"
	ndt5metrics "github.com/m-lab/ndt-server/ndt5/metrics"
	"github.com/m-lab/ndt-server/ndt5/ndt"
	"github.com/m-lab/ndt-server/ndt5/protocol"
	"github.com/m-lab/ndt-server/ndt5/singleserving"
	"github.com/m-lab/ndt-server/netx"
)

// plainServer handles requests that are TCP-based but not HTTP(S) based. If it
// receives an HTTP test it will forward that test to wsAddr, the address of the
// websocket-based server..
type plainServer struct {
	wsAddr   string
	dialer   *net.Dialer
	listener *netx.Listener
	datadir  string
	timeout  time.Duration
}

func (ps *plainServer) SingleServingServer(direction string) (ndt.SingleMeasurementServer, error) {
	return singleserving.ListenPlain(direction)
}

// sniffThenHandle implements protocol sniffing to allow WS clients and just-TCP
// clients to connect to the same port. This was a mistake to implement the
// first time, but enough clients exist that need it that we are keeping it in
// this code. In the future, if you are thinking of adding protocol sniffing to
// your system, don't.
func (ps *plainServer) sniffThenHandle(ctx context.Context, conn net.Conn) {
	// This close will frequently happen after clients have already "fled the
	// scene" after a successful test. It is an expected case that this might
	// happen after the connection has already been closed by the other side, and
	// that the Close will return an error. Therefore, avoid log spam by not using
	// warnonerror.
	defer conn.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Peek at the first three bytes. If they are "GET", then this is an HTTP
	// conversation and should be forwarded to the HTTP server.
	input := bufio.NewReader(conn)
	lead, err := input.Peek(3)
	if err != nil {
		log.Println("Could not handle connection", conn, "due to", err)
		return
	}
	if string(lead) == "GET" {
		ndt5metrics.SniffedReverseProxyCount.Inc()
		// Forward HTTP-like handshakes to the HTTP server. Note that this does NOT
		// introduce overhead for the s2c and c2s tests, because in those tests the
		// HTTP server itself opens the testing port, and that server will not use
		// this TCP proxy.
		//
		// We must forward instead of doing an HTTP redirect because existing deployed
		// clients don't support redirects, e.g.
		//    https://github.com/websockets/ws/issues/812
		fwd, err := ps.dialer.Dial("tcp", ps.wsAddr)
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
		// When the waitgroup is done, cancel the context.
		go func() {
			wg.Wait()
			cancel()
		}()
		// When the context is canceled, close `fwd` and return (returning closes
		// `conn`). Note that this cancellation could be caused by:
		//
		//   1. The context times out or is explicitly canceled, which causes fwd to
		//   close, causing each Copy() to terminate and the waitgroup.Wait() to
		//   complete.
		//    OR
		//   2. The other side of the connection closes `conn` or `fwd`, either of which
		//   causes the `Copy` operations to terminate, which causes waitgroup.Wait() to
		//   return, which cancels the context.
		//
		// No matter what happens, by the time the return executes all the above
		// goroutines should be unblocked and be either already done or in the process
		// of running to completion.
		<-ctx.Done()
		if err := ctx.Err(); err == context.DeadlineExceeded {
			log.Println("Connection", conn, "timed out")
			ndt5metrics.ClientForwardingTimeouts.Inc()
		}
		fwd.Close()
		return
	}

	// If there was no error and there was no GET, then this should be treated as a
	// legitimate attempt to perform a non-ws-based NDT test.

	// First, send the kickoff message (which is only sent for non-WS clients),
	// then transition to the protocol engine where everything should be the same
	// for plain, WS, and WSS connections.
	kickoff := "123456 654321"
	n, err := conn.Write([]byte(kickoff))
	if n != len(kickoff) || err != nil {
		log.Printf("Could not write %d byte kickoff string: %d bytes written err: %v\n", len(kickoff), n, err)
	}
	ndt5.HandleControlChannel(protocol.AdaptNetConn(conn, input), ps, "false")
}

// ListenAndServe starts up the sniffing server that delegates to the
// appropriate just-TCP or WS protocol.Connection.
func (ps *plainServer) ListenAndServe(ctx context.Context, addr string, tx Accepter) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	ps.listener = netx.NewListener(ln.(*net.TCPListener))
	// Close the listener when the context is canceled. We do this in a separate
	// goroutine to ensure that context cancellation interrupts the Accept() call.
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	// Serve requests until the context is canceled.
	go func() {
		for ctx.Err() == nil {
			conn, err := tx.Accept(ps.listener)
			if err != nil {
				log.Println("Failed to accept connection:", err)
				continue
			}
			go func() {
				connCtx, connCtxCancel := context.WithTimeout(ctx, ps.timeout)
				defer func() {
					connCtxCancel()
					r := recover()
					if r != nil {
						// TODO add a metric for this.
						log.Println("Recovered from panic in RawServer", r)
					}
				}()
				ps.sniffThenHandle(connCtx, conn)
			}()
		}
	}()
	return nil
}

func (ps *plainServer) ConnectionType() ndt.ConnectionType { return ndt.Plain }
func (ps *plainServer) DataDir() string                    { return ps.datadir }
func (ps *plainServer) LoginCeremony(conn protocol.Connection) (int, error) {
	flex, ok := conn.(protocol.MeasuredFlexibleConnection)
	if !ok {
		return 0, errors.New("the connection is unable to set its encoding dynamically - this is a bug")
	}
	v, t, err := protocol.ReadTLVMessage(conn, protocol.MsgLogin, protocol.MsgExtendedLogin)
	if err != nil {
		return 0, err
	}
	switch t {
	case protocol.MsgExtendedLogin:
		flex.SetEncoding(protocol.JSON)
		msg := protocol.JSONMessage{}
		err := json.Unmarshal(v, &msg)
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(msg.Tests)
	case protocol.MsgLogin:
		flex.SetEncoding(protocol.TLV)
		if len(v) != 1 {
			return 0, errors.New("MsgLogin requires a 1-byte message")
		}
		return int(v[0]), nil
	default:
		return 0, errors.New("Unknown message type")
	}
}

func (ps *plainServer) Addr() net.Addr {
	return ps.listener.Addr()
}

// Accepter defines an interface the listening server to decide whether to
// accept new connections.
type Accepter interface {
	Accept(l net.Listener) (net.Conn, error)
}

// Server is the interface implemented by the non-HTTP-based NDT server.
// Because it isn't run by the http.Server machinery, it has its own interface.
type Server interface {
	ListenAndServe(ctx context.Context, addr string, tx Accepter) error
	Addr() net.Addr
}

// NewServer creates a new TCP listener to serve the client. It forwards all
// connection requests that look like HTTP to a different address (assumed to be
// on the same host).
func NewServer(datadir, wsAddr string) Server {
	return &plainServer{
		wsAddr: wsAddr,
		// The dialer is only contacting localhost. The timeout should be set to a
		// small number. Resolver issues have caused connections to sometimes fail
		// when given a 10ms timeout.
		dialer: &net.Dialer{
			Timeout: 1 * time.Second,
		},
		datadir: datadir,
		// No client should wait around for more than 2 minutes.
		timeout: 2 * time.Minute,
	}
}
