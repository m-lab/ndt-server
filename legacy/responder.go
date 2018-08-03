package legacy

import (
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
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

// MakeNdtUpgrader returns a websocket.Upgrader suitable for a legacy NDT upload
// or download test.
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
