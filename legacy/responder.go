package legacy

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-cloud/ndt"
)

// Message constants for use in their respective channels
const (
	cReadyC2S = float64(-1)
	cReadyS2C = float64(-1)
)

// Responder coordinates the main control loop and subtests.
type Responder struct {
	Port   int
	Result chan float64

	kind     string
	duration time.Duration
	certFile string
	keyFile  string

	ln net.Listener
	s  *http.Server
}

// NewResponder creates a new Responder instance.
func NewResponder(kind string, duration time.Duration, certFile, keyFile string) *Responder {
	return &Responder{kind: kind, duration: duration, certFile: certFile, keyFile: keyFile}
}

// StartTLSAsync allocates a new TLS HTTP server listening on a random port. The
// server can be stopped again using TestResponder.Close().
func (tr *Responder) StartTLSAsync(mux *http.ServeMux) error {
	tr.Result = make(chan float64)
	ln, port, err := listenRandom()
	if err != nil {
		log.Println("ERROR: Failed to listen on any port:", err)
		return err
	}
	tr.Port = port
	tr.ln = ln
	tr.s = &http.Server{Handler: mux}
	go func() {
		log.Printf("%s: Serving for test on %s", tr.kind, ln.Addr())
		err := tr.s.ServeTLS(ln, tr.certFile, tr.keyFile)
		if err != nil && err != http.ErrServerClosed {
			log.Printf("ERROR: %s Starting TLS server: %s", tr.kind, err)
		}
	}()
	return nil
}

// C2STestHandler is an http.Handler that executes the NDT c2s test over websockets.
func (tr *Responder) C2STestHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := makeNdtUpgrader([]string{"c2s"})
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade should have already returned an HTTP error code.
		log.Println("ERROR C2S: upgrader", err)
		return
	}
	defer ws.Close()

	// Define an absolute deadline for running all tests.
	deadline := time.Now().Add(tr.duration)

	// Signal ready, and run the test.
	tr.Result <- cReadyC2S
	bytesPerSecond := runC2S(ws, deadline.Sub(time.Now()), true)
	tr.Result <- bytesPerSecond

	// Drain client for a few more seconds, and discard results.
	_ = runC2S(ws, deadline.Sub(time.Now()), false)
}

// C2SController manages communication with the C2STestHandler from the control
// channel.
func (tr *Responder) C2SController(ws *websocket.Conn) (float64, error) {
	// Wait for test to run.
	// Send the server port to the client.
	SendNdtMessage(ndt.TestPrepare, strconv.Itoa(tr.Port), ws)
	c2sReady := <-tr.Result
	if c2sReady != cReadyC2S {
		return 0, fmt.Errorf("ERROR C2S: Bad value received on the c2s channel")
	}
	SendNdtMessage(ndt.TestStart, "", ws)
	c2sBytesPerSecond := <-tr.Result
	c2sKbps := 8 * c2sBytesPerSecond / 1000.0

	SendNdtMessage(ndt.TestMsg, fmt.Sprintf("%.4f", c2sKbps), ws)
	SendNdtMessage(ndt.TestFinalize, "", ws)
	log.Println("C2S: server rate:", c2sKbps)
	return c2sKbps, nil
}

// Close will shutdown, cancel, or close all resources used by the test.
func (tr *Responder) Close() {
	log.Printf("Closing %s Responder", tr.kind)
	if tr.s != nil {
		// Shutdown the server for the test.
		tr.s.Close()
	}
	if tr.ln != nil {
		// Shutdown the socket listener.
		tr.ln.Close()
	}
	// Close channel for communication between the control routine and test routine.
	close(tr.Result)
}

// runC2S performs a 10 second NDT client to server test. Runtime is
// guaranteed to be no more than timeout. The timeout should be slightly greater
// than 10 sec. The given websocket should be closed by the caller.
func runC2S(ws *websocket.Conn, timeout time.Duration, logErrors bool) float64 {
	done := make(chan float64)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Run recv in background.
	go func() {
		bytesPerSec, err := recvUntil(ws, 10*time.Second)
		if err != nil {
			cancel()
			if logErrors {
				log.Println("C2S: recvUntil error:", err)
			}
			return
		}
		done <- bytesPerSec
	}()

	select {
	case <-ctx.Done():
		if logErrors {
			log.Println("C2S: Context Done!", ctx.Err())
		}
		// Return zero on error.
		return 0
	case bytesPerSecond := <-done:
		return bytesPerSecond
	}
}

// recvUntil reads from the given websocket for duration seconds and returns the
// average rate.
func recvUntil(ws *websocket.Conn, duration time.Duration) (float64, error) {
	totalBytes := float64(0)
	startTime := time.Now()
	endTime := startTime.Add(duration)
	for time.Now().Before(endTime) {
		_, buffer, err := ws.ReadMessage()
		if err != nil {
			return 0, err
		}
		totalBytes += float64(len(buffer))
	}
	bytesPerSecond := totalBytes / float64(time.Since(startTime)/time.Second)
	return bytesPerSecond, nil
}

func makeNdtUpgrader(protocols []string) websocket.Upgrader {
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
	return tcpKeepAliveListener{ln}, port, nil
}

// Note: Copied from net/http package.
// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}
