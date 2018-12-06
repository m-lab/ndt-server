package ndt

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-cloud/ndt/protocol"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	cTestC2S    = 2
	cTestS2C    = 4
	cTestStatus = 16
)

var (
	testRate = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "ndt_test_rate_mbps",
			Help: "A histogram of request rates.",
			Buckets: []float64{
				1, 1.5, 2.5, 4, 6,
				10, 15, 25, 40, 60,
				100, 150, 250, 400, 600,
				1000},
		},
		[]string{"direction"},
	)
	testCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_test_total",
			Help: "Number of NDT tests run by this server.",
		},
		[]string{"direction", "code"},
	)
)

func init() {
	prometheus.MustRegister(testCount)
	prometheus.MustRegister(testRate)
}

// Message constants for use in their respective channels
const (
	cReadyC2S = float64(-1)
	cReadyS2C = float64(-1)
)

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

// TestResponder coordinates synchronization between the main control loop and subtests.
type TestResponder struct {
	response chan float64
	port     int
	ln       net.Listener
	s        *http.Server
	ctx      context.Context
	cancel   context.CancelFunc
}

// StartTLSAsync allocates a new TLS HTTP server listening on a random port. The
// server can be stopped again using TestResponder.Close().
func (tr *TestResponder) StartTLSAsync(mux *http.ServeMux, msg, certFile, keyFile string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	tr.ctx = ctx
	tr.cancel = cancel
	tr.response = make(chan float64)
	ln, port, err := listenRandom()
	if err != nil {
		log.Println("ERROR: Failed to listen on any port:", err)
		return err
	}
	tr.port = port
	tr.ln = ln
	tr.s = &http.Server{Handler: mux}
	go func() {
		log.Printf("%s: Serving for test on %s", msg, ln.Addr())
		err := tr.s.ServeTLS(ln, certFile, keyFile)
		if err != nil && err != http.ErrServerClosed {
			log.Printf("ERROR: %s Starting TLS server: %s", msg, err)
		}
	}()
	return nil
}

// Port returns the random port assigned to the TestResponder server. Must be
// called after StartTLSAsync.
func (tr *TestResponder) Port() int {
	return tr.port
}

// Close will shutdown, cancel, or close all resources used by the test.
func (tr *TestResponder) Close() {
	log.Println("Closing Test Responder")
	if tr.s != nil {
		// Shutdown the server for the test.
		tr.s.Close()
	}
	if tr.ln != nil {
		// Shutdown the socket listener.
		tr.ln.Close()
	}
	if tr.cancel != nil {
		// Cancel the test responder context.
		tr.cancel()
	}
	// Close channel for communication between the control routine and test routine.
	close(tr.response)
}

// Server contains everything needed to start a new server on a random port.
type Server struct {
	CertFile string
	KeyFile  string
}

// Listen on a random port.
func listenRandom() (net.Listener, int, error) {
	// Start listening
	ln, err := net.ListenTCP("tcp", &net.TCPAddr{})
	if err != nil {
		return nil, 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	return RawListener{TCPListener: ln, TryToEnableBBR: false}, port, nil
}

// TODO: run meta test.
func runMetaTest(ws *websocket.Conn) {
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

// ServeHTTP is the command channel for the NDT-WS test. All subsequent client
// communication is synchronized with this method. Returning closes the
// websocket connection, so only occurs after all tests complete or an
// unrecoverable error.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upgrader := makeNdtUpgrader([]string{"ndt"})
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ERROR SERVER:", err)
		return
	}
	defer ws.Close()

	message, err := protocol.ReceiveJSONMessage(ws, protocol.MsgExtendedLogin)
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
		log.Println("We don't support clients that don't support cTestStatus")
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

	protocol.SendJSONMessage(protocol.SrvQueue, "0", ws)
	protocol.SendJSONMessage(protocol.MsgLogin, "v5.0-NDTinGO", ws)
	protocol.SendJSONMessage(protocol.MsgLogin, strings.Join(testsToRun, " "), ws)

	var c2sRate, s2cRate float64
	if runC2s {
		c2sRate, err = s.manageC2sTest(ws)
		if err != nil {
			log.Println("ERROR: manageC2sTest", err)
		} else {
			testRate.WithLabelValues("c2s").Observe(c2sRate / 1000.0)
		}
	}
	if runS2c {
		s2cRate, err = s.manageS2cTest(ws)
		if err != nil {
			log.Println("ERROR: manageS2cTest", err)
		} else {
			testRate.WithLabelValues("s2c").Observe(s2cRate / 1000.0)
		}
	}
	log.Printf("NDT: %s uploaded at %.4f and downloaded at %.4f", r.RemoteAddr, c2sRate, s2cRate)
	protocol.SendJSONMessage(protocol.MsgResults, fmt.Sprintf("You uploaded at %.4f and downloaded at %.4f", c2sRate, s2cRate), ws)
	protocol.SendJSONMessage(protocol.MsgLogout, "", ws)
}
