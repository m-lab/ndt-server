package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-cloud/legacy"
	"github.com/m-lab/ndt-cloud/ndt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Message constants for the NDT protocol
// Message constants for use in their respective channels
const (
	cReadyC2S = float64(-1)
	cReadyS2C = float64(-1)
)

// Flags that can be passed in on the command line
var (
	fNdtPort     = flag.String("port", "3010", "The port to use for the main NDT test")
	fCertFile    = flag.String("cert", "", "The file with server certificates in PEM format.")
	fKeyFile     = flag.String("key", "", "The file with server key in PEM format.")
	fMetricsAddr = flag.String("metrics_address", ":9090", "Export prometheus metrics on this address and port.")
)

var (
	currentTests = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ndt_control_current",
		Help: "A gauge of requests currently being served by the NDT control handler.",
	})
	testDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "ndt_control_duration_seconds",
			Help: "A histogram of request latencies to the control channel.",
			// Durations will likely be tri-modal: early failures (fast),
			// completed single test (slower), completed dual tests (slowest) or timeouts.
			Buckets: []float64{.1, 1, 10, 10.5, 11, 11.5, 12, 20, 21, 22, 30, 60},
		},
		[]string{"code"},
	)
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
	lameDuck = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "lame_duck_experiment",
		Help: "Indicates when the server is in lame duck",
	})
)

func init() {
	prometheus.MustRegister(currentTests)
	prometheus.MustRegister(testDuration)
	prometheus.MustRegister(testCount)
	prometheus.MustRegister(testRate)
	prometheus.MustRegister(lameDuck)
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

// S2CTestServer performs the NDT s2c test.
func (tr *TestResponder) S2CTestServer(w http.ResponseWriter, r *http.Request) {
	upgrader := makeNdtUpgrader([]string{"s2c"})
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade should have already returned an HTTP error code.
		log.Println("ERROR S2C: upgrader", err)
		return
	}
	defer ws.Close()
	dataToSend := make([]byte, 81920)
	for i := range dataToSend {
		dataToSend[i] = byte(((i * 101) % (122 - 33)) + 33)
	}
	messageToSend, err := websocket.NewPreparedMessage(websocket.BinaryMessage, dataToSend)
	if err != nil {
		log.Println("ERROR S2C: Could not make prepared message:", err)
		return
	}

	// Signal control channel that we are about to start the test.
	tr.response <- cReadyS2C
	tr.response <- tr.sendS2CUntil(ws, messageToSend, len(dataToSend))
}

func (tr *TestResponder) sendS2CUntil(ws *websocket.Conn, msg *websocket.PreparedMessage, dataLen int) float64 {
	// Create ticker to enforce timeout on
	done := make(chan float64)

	go func() {
		totalBytes := float64(0)
		startTime := time.Now()
		endTime := startTime.Add(10 * time.Second)
		for time.Now().Before(endTime) {
			err := ws.WritePreparedMessage(msg)
			if err != nil {
				log.Println("ERROR S2C: sending message", err)
				tr.cancel()
				return
			}
			totalBytes += float64(dataLen)
		}
		done <- totalBytes / float64(time.Since(startTime)/time.Second)
	}()

	log.Println("S2C: Waiting for test to complete or timeout")
	select {
	case <-tr.ctx.Done():
		log.Println("S2C: Context Done!", tr.ctx.Err())
		ws.Close()
		// Return zero on error.
		return 0
	case bytesPerSecond := <-done:
		return bytesPerSecond
	}
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

func manageC2sTest(ws *websocket.Conn) (float64, error) {
	// Create a testResponder instance.
	testResponder := legacy.NewResponder("C2S", 20*time.Second, *fCertFile, *fKeyFile)

	// Create a TLS server for running the C2S test.
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/ndt_protocol",
		promhttp.InstrumentHandlerCounter(
			testCount.MustCurryWith(prometheus.Labels{"direction": "c2s"}),
			http.HandlerFunc(testResponder.C2STestHandler)))
	err := testResponder.StartTLSAsync(serveMux)
	if err != nil {
		return 0, err
	}
	defer testResponder.Close()

	done := make(chan float64)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	go func() {
		c2sKbps, err := testResponder.C2SController(ws)
		if err != nil {
			cancel()
			log.Println("C2S: C2SController error:", err)
			return
		}
		done <- c2sKbps
	}()

	select {
	case <-ctx.Done():
		log.Println("C2S: ctx Done!")
		return 0, ctx.Err()
	case value := <-done:
		log.Println("C2S: finished ", value)
		return value, nil
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

// StartTLSAsync allocates a new TLS HTTP server listening on a random port. The
// server can be stopped again using TestResponder.Close().
func (tr *TestResponder) StartTLSAsync(mux *http.ServeMux, msg string) error {
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
		err := tr.s.ServeTLS(ln, *fCertFile, *fKeyFile)
		if err != nil && err != http.ErrServerClosed {
			log.Printf("ERROR: %s Starting TLS server: %s", msg, err)
		}
	}()
	return nil
}

func manageS2cTest(ws *websocket.Conn) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Create a testResponder instance.
	testResponder := &TestResponder{}

	// Create a TLS server for running the S2C test.
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/ndt_protocol",
		promhttp.InstrumentHandlerCounter(
			testCount.MustCurryWith(prometheus.Labels{"direction": "s2c"}),
			http.HandlerFunc(testResponder.S2CTestServer)))
	err := testResponder.StartTLSAsync(serveMux, "S2C")
	if err != nil {
		return 0, err
	}
	defer testResponder.Close()

	done := make(chan float64)
	go func() {
		// Wait for test to run. ///////////////////////////////////////////
		// Send the server port to the client.
		legacy.SendNdtMessage(ndt.TestPrepare, strconv.Itoa(testResponder.Port()), ws)
		s2cReady := <-testResponder.response
		if s2cReady != cReadyS2C {
			log.Println("ERROR S2C: Bad value received on the s2c channel", s2cReady)
			cancel()
			return
		}
		legacy.SendNdtMessage(ndt.TestStart, "", ws)
		s2cBytesPerSecond := <-testResponder.response
		s2cKbps := 8 * s2cBytesPerSecond / 1000.0

		// Send additional download results to the client.
		resultMsg := &legacy.NdtS2CResult{
			ThroughputValue:  s2cKbps,
			UnsentDataAmount: 0,
			TotalSentByte:    int64(10 * s2cBytesPerSecond), // TODO: use actual bytes sent.
		}
		err = legacy.WriteNdtMessage(ws, ndt.TestMsg, resultMsg)
		if err != nil {
			log.Println("S2C: Failed to write JSON message:", err)
			cancel()
			return
		}
		clientRateMsg, err := legacy.RecvNdtJSONMessage(ws, ndt.TestMsg)
		if err != nil {
			log.Println("S2C: Failed to read JSON message:", err)
			cancel()
			return
		}
		log.Println("S2C: The client sent us:", clientRateMsg.Msg)
		requiredWeb100Vars := []string{"MaxRTT", "MinRTT"}

		for _, web100Var := range requiredWeb100Vars {
			legacy.SendNdtMessage(ndt.TestMsg, web100Var+": 0", ws)
		}
		legacy.SendNdtMessage(ndt.TestFinalize, "", ws)
		clientRate, err := strconv.ParseFloat(clientRateMsg.Msg, 64)
		if err != nil {
			log.Println("S2C: Bad client rate:", err)
			cancel()
			return
		}
		log.Println("S2C: server rate:", s2cKbps, "vs client rate:", clientRate)
		done <- s2cKbps
	}()

	select {
	case <-ctx.Done():
		log.Println("S2C: ctx done!")
		return 0, ctx.Err()
	case rate := <-done:
		log.Println("S2C: finished ", rate)
		return rate, nil
	}
}

// TODO: run meta test.
func runMetaTest(ws *websocket.Conn) {
	var err error
	var message *legacy.NdtJSONMessage

	legacy.SendNdtMessage(ndt.TestPrepare, "", ws)
	legacy.SendNdtMessage(ndt.TestStart, "", ws)
	for {
		message, err = legacy.RecvNdtJSONMessage(ws, ndt.TestMsg)
		if message.Msg == "" || err != nil {
			break
		}
		log.Println("Meta message: ", message)
	}
	if err != nil {
		log.Println("Error reading JSON message:", err)
		return
	}
	legacy.SendNdtMessage(ndt.TestFinalize, "", ws)
}

// NdtServer is the command channel for the NDT-WS test. All subsequent client
// communication is synchronized with this method. Returning closes the
// websocket connection, so only occurs after all tests complete or an
// unrecoverable error.
func NdtServer(w http.ResponseWriter, r *http.Request) {
	upgrader := makeNdtUpgrader([]string{"ndt"})
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ERROR SERVER:", err)
		return
	}
	defer ws.Close()

	message, err := legacy.RecvNdtJSONMessage(ws, ndt.MsgExtendedLogin)
	if err != nil {
		log.Println("Error reading JSON message:", err)
		return
	}
	tests, err := strconv.ParseInt(message.Tests, 10, 64)
	if err != nil {
		log.Println("Failed to parse Tests integer:", err)
		return
	}
	if (tests & ndt.TestStatus) == 0 {
		log.Println("We don't support clients that don't support cTestStatus")
		return
	}
	testsToRun := []string{}
	runC2s := (tests & ndt.TestC2S) != 0
	runS2c := (tests & ndt.TestS2C) != 0

	if runC2s {
		testsToRun = append(testsToRun, strconv.Itoa(ndt.TestC2S))
	}
	if runS2c {
		testsToRun = append(testsToRun, strconv.Itoa(ndt.TestS2C))
	}

	legacy.SendNdtMessage(ndt.SrvQueue, "0", ws)
	legacy.SendNdtMessage(ndt.MsgLogin, "v5.0-NDTinGO", ws)
	legacy.SendNdtMessage(ndt.MsgLogin, strings.Join(testsToRun, " "), ws)

	var c2sRate, s2cRate float64
	if runC2s {
		c2sRate, err = manageC2sTest(ws)
		if err != nil {
			log.Println("ERROR: manageC2sTest", err)
		} else {
			testRate.WithLabelValues("c2s").Observe(c2sRate / 1000.0)
		}
	}
	if runS2c {
		s2cRate, err = manageS2cTest(ws)
		if err != nil {
			log.Println("ERROR: manageS2cTest", err)
		} else {
			testRate.WithLabelValues("s2c").Observe(s2cRate / 1000.0)
		}
	}
	log.Printf("NDT: %s uploaded at %.4f and downloaded at %.4f", r.RemoteAddr, c2sRate, s2cRate)
	legacy.SendNdtMessage(ndt.MsgResults, fmt.Sprintf("You uploaded at %.4f and downloaded at %.4f", c2sRate, s2cRate), ws)
	legacy.SendNdtMessage(ndt.MsgLogout, "", ws)
}

func defaultHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(`
This is an NDT server.

It only works with Websockets and SSL.

You can run a test here: :3010/static/widget.html
You can monitor its status on port :9090/metrics.
`))
}

func catchSigterm() {
	// Disable lame duck status.
	lameDuck.Set(0)

	// Register channel to receive SIGTERM events.
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)

	for {
		// Wait until we receive a SIGTERM.
		fmt.Println("Received signal:", <-c)
		// Set lame duck status. This will remain set until exit.
		lameDuck.Set(1)
	}
}

func main() {
	flag.Parse()
	go catchSigterm()
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		mux.Handle("/metrics", promhttp.Handler())
		log.Fatal(http.ListenAndServe(*fMetricsAddr, mux))
	}()

	http.HandleFunc("/", defaultHandler)
	http.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
	http.Handle("/ndt_protocol",
		promhttp.InstrumentHandlerInFlight(currentTests,
			promhttp.InstrumentHandlerDuration(testDuration,
				http.HandlerFunc(NdtServer))))

	log.Println("About to listen on " + *fNdtPort + ". Go to http://127.0.0.1:" + *fNdtPort + "/")
	log.Fatal(http.ListenAndServeTLS(":"+*fNdtPort, *fCertFile, *fKeyFile, nil))
}
