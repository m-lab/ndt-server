package legacy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/m-lab/ndt-server/legacy/c2s"
	legacymetrics "github.com/m-lab/ndt-server/legacy/metrics"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/s2c"
	"github.com/m-lab/ndt-server/legacy/testresponder"
	"github.com/m-lab/ndt-server/metrics"
)

const (
	cTestC2S    = 2
	cTestS2C    = 4
	cTestStatus = 16
)

// BasicServer contains everything needed to start a new server on a random port.
type BasicServer struct {
	CertFile       string
	KeyFile        string
	ServerType     testresponder.ServerType
	ForwardingAddr string
	Labels         prometheus.Labels
}

// TODO: run meta test.
func runMetaTest(ws protocol.Connection) {
	var err error
	var message *protocol.JSONMessage

	protocol.SendJSONMessage(protocol.TestPrepare, "", ws)
	protocol.SendJSONMessage(protocol.TestStart, "", ws)
	for {
		message, err = protocol.ReceiveJSONMessage(ws, protocol.TestMsg)
		if message.Msg == "" || err != nil {
			break
		}
		log.Println("Meta message: ", message)
	}
	if err != nil {
		log.Println("Error reading JSON message:", err)
		return
	}
	protocol.SendJSONMessage(protocol.TestFinalize, "", ws)
}

// ServeHTTP is the command channel for the NDT-WS or NDT-WSS test. All
// subsequent client communication is synchronized with this method. Returning
// closes the websocket connection, so only occurs after all tests complete or
// an unrecoverable error. It is called ServeHTTP to make sure that the Server
// implements the http.Handler interface.
func (s *BasicServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upgrader := testresponder.MakeNdtUpgrader([]string{"ndt"})
	wsc, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ERROR SERVER:", err)
		return
	}
	ws := protocol.AdaptWsConn(wsc)
	defer ws.Close()
	s.handleControlChannel(ws)
}

// handleControlChannel is the "business logic" of an NDT test. It is designed
// to run every test, and to never need to know whether the underlying
// connection is just a TCP socket, a WS connection, or a WSS connection.
func (s *BasicServer) handleControlChannel(conn protocol.Connection) {
	config := &testresponder.Config{
		ServerType: s.ServerType,
		CertFile:   s.CertFile,
		KeyFile:    s.KeyFile,
	}

	message, err := protocol.ReceiveJSONMessage(conn, protocol.MsgExtendedLogin)
	if err != nil {
		log.Println("Error reading JSON message:", err)
		return
	}
	tests, err := strconv.ParseInt(message.Tests, 10, 64)
	if err != nil {
		log.Println("Failed to parse Tests integer:", err)
		return
	}
	if (tests & cTestStatus) == 0 {
		log.Println("We don't support clients that don't support TestStatus")
		return
	}
	testsToRun := []string{}
	runC2s := (tests & cTestC2S) != 0
	runS2c := (tests & cTestS2C) != 0

	if runC2s {
		testsToRun = append(testsToRun, strconv.Itoa(cTestC2S))
	}
	if runS2c {
		testsToRun = append(testsToRun, strconv.Itoa(cTestS2C))
	}

	protocol.SendJSONMessage(protocol.SrvQueue, "0", conn)
	protocol.SendJSONMessage(protocol.MsgLogin, "v5.0-NDTinGO", conn)
	protocol.SendJSONMessage(protocol.MsgLogin, strings.Join(testsToRun, " "), conn)

	var c2sRate, s2cRate float64
	if runC2s {
		c2sRate, err = c2s.ManageTest(conn, config)
		if err != nil {
			log.Println("ERROR: manageC2sTest", err)
		} else {
			legacymetrics.TestRate.WithLabelValues("c2s").Observe(c2sRate / 1000.0)
		}
	}
	if runS2c {
		s2cRate, err = s2c.ManageTest(conn, config)
		if err != nil {
			log.Println("ERROR: manageS2cTest", err)
		} else {
			legacymetrics.TestRate.WithLabelValues("s2c").Observe(s2cRate / 1000.0)
		}
	}
	log.Printf("NDT: uploaded at %.4f and downloaded at %.4f", c2sRate, s2cRate)
	protocol.SendJSONMessage(protocol.MsgResults, fmt.Sprintf("You uploaded at %.4f and downloaded at %.4f", c2sRate, s2cRate), conn)
	protocol.SendJSONMessage(protocol.MsgLogout, "", conn)

}

// sniffThenHandle implements protocol sniffing to allow WS clients and just-TCP
// clients to connect to the same port. This was a mistake to implement the
// first time, but enough clients exist that need it that we are keeping it in
// this code. In the future, if you are thinking of adding protocol sniffing to
// your system, don't.
func (s *BasicServer) sniffThenHandle(conn net.Conn) {
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
		log.Println("Could not handle connection", conn)
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
		fwd, err := net.Dial("tcp", s.ForwardingAddr)
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
	s.handleControlChannel(protocol.AdaptNetConn(conn, input))
}

// ListenAndServeRawAsync starts up the sniffing server that delegates to the
// appropriate just-TCP or WS protocol.Connection.
func (s *BasicServer) ListenAndServeRawAsync(ctx context.Context, addr string) error {
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
				s.sniffThenHandle(conn)
			}()
		}
	}()
	return nil
}
