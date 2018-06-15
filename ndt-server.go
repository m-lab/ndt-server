package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Message constants for the NDT protocol
const (
	SrvQueue         = byte(1)
	MsgLogin         = byte(2)
	TestPrepare      = byte(3)
	TestStart        = byte(4)
	TestMsg          = byte(5)
	TestFinalize     = byte(6)
	MsgError         = byte(7)
	MsgResults       = byte(8)
	MsgLogout        = byte(9)
	MsgWaiting       = byte(10)
	MsgExtendedLogin = byte(11)

	TEST_C2S    = 2
	TEST_S2C    = 4
	TEST_STATUS = 16
)

// Message constants for use in their respective channels
const (
	C2sReady = float64(-1)
	S2cReady = float64(-1)
)

// Flags that can be passed in on the command line
var (
	NdtPort  = flag.String("port", "3010", "The port to use for the main NDT test")
	certFile = flag.String("cert", "", "The file with server certificates in PEM format.")
	keyFile  = flag.String("key", "", "The file with server key in PEM format.")
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
			// completed single test (slower), completed dual tests (slowest).
			Buckets: []float64{.1, 1, 10, 11, 12, 13, 14, 15, 17, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 60, 180},
		},
		[]string{"code"},
	)
	testRate = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "ndt_test_rate_mbps",
			Help: "A histogram of request rates.",
			Buckets: []float64{
				1, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55, 60, 65, 70, 75, 80, 85, 90, 95, 100,
				110, 120, 130, 140, 150, 160, 170, 180, 190, 200,
				220, 240, 260, 280, 300,
				333, 366, 400,
				450, 500,
				600, 700, 800, 900, 1000},
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
	prometheus.MustRegister(currentTests)
	prometheus.MustRegister(testDuration)
	prometheus.MustRegister(testCount)
	prometheus.MustRegister(testRate)
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

func readNdtMessage(ws *websocket.Conn, expectedType byte) ([]byte, error) {
	_, buffer, err := ws.ReadMessage()
	if err != nil {
		return nil, err
	}
	if buffer[0] != expectedType {
		return nil, fmt.Errorf("Read wrong message type. Wanted 0x%x, got 0x%x", expectedType, buffer[0])
	}
	// TODO: what's in bytes [1:3]?
	return buffer[3:], nil
}

func writeNdtMessage(ws *websocket.Conn, msgType byte, msg fmt.Stringer) error {
	message := msg.String()
	outbuff := make([]byte, 3+len(message))
	outbuff[0] = msgType
	outbuff[1] = byte((len(message) >> 8) & 0xFF)
	outbuff[2] = byte(len(message) & 0xFF)
	for i := range message {
		outbuff[i+3] = message[i]
	}
	return ws.WriteMessage(websocket.BinaryMessage, outbuff)
}

// NdtJSONMessage holds the JSON messages we can receive from the server. We
// only support the subset of the NDT JSON protocol that has two fields: msg,
// and tests.
type NdtJSONMessage struct {
	Msg   string `json:"msg"`
	Tests string `json:"tests,omitempty"`
}

func (n *NdtJSONMessage) String() string {
	b, _ := json.Marshal(n)
	return string(b)
}

// NdtS2CResult is the result object returned to S2C clients as JSON.
type NdtS2CResult struct {
	ThroughputValue  float64
	UnsentDataAmount int64
	TotalSentByte    int64
}

func (n *NdtS2CResult) String() string {
	b, _ := json.Marshal(n)
	return string(b)
}

func recvNdtJSONMessage(ws *websocket.Conn, expectedType byte) (*NdtJSONMessage, error) {
	message := &NdtJSONMessage{}
	jsonString, err := readNdtMessage(ws, expectedType)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(jsonString, &message)
	if err != nil {
		return nil, err
	}
	return message, nil
}

func sendNdtMessage(msgType byte, msg string, ws *websocket.Conn) error {
	message := &NdtJSONMessage{Msg: msg}
	return writeNdtMessage(ws, msgType, message)
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
}

// S2CTestServer performs the NDT s2c test.
func (tr *TestResponder) S2CTestServer(w http.ResponseWriter, r *http.Request) {
	upgrader := makeNdtUpgrader([]string{"s2c"})
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade should have already returned an HTTP error code.
		log.Println(tr.port, "ERROR S2C: upgrader", err)
		return
	}
	defer ws.Close()
	dataToSend := make([]byte, 81920)
	for i := range dataToSend {
		dataToSend[i] = byte(((i * 101) % (122 - 33)) + 33)
	}
	messageToSend, err := websocket.NewPreparedMessage(websocket.BinaryMessage, dataToSend)
	if err != nil {
		log.Println(tr.port, "ERROR S2C: Could not make prepared message:", err)
		return
	}

	// Signal control channel that we are about to start the test.
	tr.response <- S2cReady

	// Create ticker to enforce timeout on
	ticker := time.NewTicker(11 * time.Second)
	defer ticker.Stop()
	done := make(chan float64)

	go func() {
		totalBytes := float64(0)
		log.Println(tr.port, "S2C: Test starts at", time.Now().Format(time.RFC3339), "and ends in", (10 * time.Second))
		startTime := time.Now()
		endTime := startTime.Add(10 * time.Second)
		for time.Now().Before(endTime) {
			err := ws.WritePreparedMessage(messageToSend)
			if err != nil {
				log.Println(tr.port, "ERROR S2C: sending message", err)
				done <- -1
				return
			}
			totalBytes += float64(len(dataToSend))
		}
		done <- totalBytes / float64(time.Since(startTime)/time.Second)
	}()

	log.Println(tr.port, "S2C: Waiting for test to complete or timeout")
	select {
	case bytesPerSecond := <-done:
		tr.response <- bytesPerSecond
		break
	case <-ticker.C:
		tr.response <- -1
		break
	}
}

func recvC2SUntil(ws *websocket.Conn, d time.Duration) float64 {
	ticker := time.NewTicker(d + time.Second)
	defer ticker.Stop()
	done := make(chan float64)

	go func() {
		totalBytes := float64(0)
		startTime := time.Now()
		endTime := startTime.Add(d)
		i := 0
		for time.Now().Before(endTime) {
			_, buffer, err := ws.ReadMessage()
			if err != nil {
				done <- -1
				return
			}
			totalBytes += float64(len(buffer))
			i++
		}
		bytesPerSecond := totalBytes / float64(time.Since(startTime)/time.Second)
		done <- bytesPerSecond
	}()

	log.Println(ws.RemoteAddr().String(), "C2S: Waiting for reader or ticker")
	select {
	case bytesPerSecond := <-done:
		return bytesPerSecond
	case <-ticker.C:
		return -1
	}
}

// C2STestServer performs the NDT c2s test.
func (tr *TestResponder) C2STestServer(w http.ResponseWriter, r *http.Request) {
	upgrader := makeNdtUpgrader([]string{"c2s"})
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade should have already returned an HTTP error code.
		log.Println(tr.port, "ERROR C2S: upgrader", err)
		return
	}
	defer ws.Close()
	tr.response <- C2sReady
	bytesPerSecond := recvC2SUntil(ws, 10*time.Second)
	tr.response <- bytesPerSecond

	// Drain client for a few more seconds, and discard results.
	ts := time.Now()
	_ = recvC2SUntil(ws, 4*time.Second)
	log.Println(tr.port, "C2S: wait time", time.Now().Sub(ts))
}

func manageC2sTest(ws *websocket.Conn) float64 {
	// Open the socket
	serveMux := http.NewServeMux()
	testResponder := &TestResponder{
		response: make(chan float64),
	}
	serveMux.HandleFunc("/ndt_protocol",
		promhttp.InstrumentHandlerCounter(
			testCount.MustCurryWith(prometheus.Labels{"direction": "c2s"}),
			http.HandlerFunc(testResponder.C2STestServer)))
	// Start listening
	s := http.Server{Handler: serveMux}
	defer s.Close()

	ln, err := net.ListenTCP("tcp", &net.TCPAddr{})
	if err != nil {
		log.Println(ws.RemoteAddr(), "ERROR C2S: Failed to listen on any port:", err)
		return -1
	}
	defer ln.Close()
	c2sServerPort := ln.Addr().(*net.TCPAddr).Port
	go func() {
		log.Println(c2sServerPort, "C2S: About to listen for C2S on", ln.Addr())
		s.ServeTLS(tcpKeepAliveListener{ln}, *certFile, *keyFile)
	}()
	testResponder.port = c2sServerPort

	// Tell the client to go
	sendNdtMessage(TestPrepare, strconv.Itoa(c2sServerPort), ws)
	c2sReady := <-testResponder.response
	if c2sReady != C2sReady {
		log.Println(c2sServerPort, "ERROR C2S: Bad value received on the c2s channel", c2sReady)
		return -1
	}
	sendNdtMessage(TestStart, "", ws)
	c2sRate := <-testResponder.response
	sendNdtMessage(TestMsg, fmt.Sprintf("%.4f", c2sRate), ws)
	sendNdtMessage(TestFinalize, "", ws)
	return c2sRate
}

func manageS2cTest(ws *websocket.Conn) float64 {
	// Open the socket
	serveMux := http.NewServeMux()
	testResponder := &TestResponder{
		response: make(chan float64),
	}
	serveMux.HandleFunc("/ndt_protocol",
		promhttp.InstrumentHandlerCounter(
			testCount.MustCurryWith(prometheus.Labels{"direction": "s2c"}),
			http.HandlerFunc(testResponder.S2CTestServer)))
	// Start listening
	s := http.Server{Handler: serveMux}
	defer s.Close()
	ln, err := net.ListenTCP("tcp", &net.TCPAddr{})
	if err != nil {
		log.Println("S2C: Failed to listen on any port:", err)
		return -1
	}
	defer ln.Close()
	s2cServerPort := ln.Addr().(*net.TCPAddr).Port
	testResponder.port = s2cServerPort
	go func() {
		log.Println(s2cServerPort, "S2C: About to listen for S2C on", s2cServerPort)
		s.ServeTLS(tcpKeepAliveListener{ln}, *certFile, *keyFile)
	}()

	// Tell the client to go
	sendNdtMessage(TestPrepare, strconv.Itoa(int(s2cServerPort)), ws)
	s2cReady := <-testResponder.response
	if s2cReady != S2cReady {
		log.Println(s2cServerPort, "S2C: Bad value received on the s2c channel", s2cReady)
		return -1
	}
	sendNdtMessage(TestStart, "", ws)
	s2cBytesPerSecond := <-testResponder.response
	s2cKbps := 8 * s2cBytesPerSecond / 1000.0

	resultMsg := &NdtS2CResult{
		ThroughputValue:  s2cKbps,
		UnsentDataAmount: 0,
		TotalSentByte:    int64(10 * s2cBytesPerSecond), // TODO: use actual bytes sent.
	}
	err = writeNdtMessage(ws, TestMsg, resultMsg)
	if err != nil {
		log.Println(s2cServerPort, "S2C: Failed to write JSON message:", err)
		return -1
	}
	clientRateMsg, err := recvNdtJSONMessage(ws, TestMsg)
	if err != nil {
		log.Println(s2cServerPort, "S2C: Failed to read JSON message:", err)
		return -1
	}
	log.Println(s2cServerPort, "S2C: The client sent us:", clientRateMsg.Msg)
	requiredWeb100Vars := []string{"AckPktsIn", "CountRTT", "CongestionSignals", "CurRTO", "CurMSS",
		"DataBytesOut", "DupAcksIn", "MaxCwnd", "MaxRwinRcvd", "PktsOut", "PktsRetrans", "RcvWinScale",
		"Sndbuf", "SndLimTimeCwnd", "SndLimTimeRwin", "SndLimTimeSender", "SndWinScale", "SumRTT", "Timeouts",
		"MaxRTT", "MinRTT"}

	for _, web100Var := range requiredWeb100Vars {
		sendNdtMessage(TestMsg, web100Var+": 0", ws)
	}
	sendNdtMessage(TestFinalize, "", ws)
	clientRate, err := strconv.ParseFloat(clientRateMsg.Msg, 64)
	if err != nil {
		log.Println(s2cServerPort, "S2C: Bad client rate:", err)
		return -1
	}
	log.Println(s2cServerPort, "S2C: client rate:", clientRate, "vs", s2cKbps)
	return s2cKbps
}

// TODO: run meta test.
func runMetaTest(ws *websocket.Conn) {
	var err error
	var message *NdtJSONMessage

	sendNdtMessage(TestPrepare, "", ws)
	sendNdtMessage(TestStart, "", ws)
	for {
		message, err = recvNdtJSONMessage(ws, TestMsg)
		if message.Msg == "" || err != nil {
			break
		}
		log.Println("Meta message: ", message)
	}
	if err != nil {
		log.Println("Error reading JSON message:", err)
		return
	}
	sendNdtMessage(TestFinalize, "", ws)
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

	message, err := recvNdtJSONMessage(ws, MsgExtendedLogin)
	if err != nil {
		log.Println("Error reading JSON message:", err)
		return
	}
	tests, err := strconv.ParseInt(message.Tests, 10, 64)
	if err != nil {
		log.Println("Failed to parse Tests integer:", err)
		return
	}
	if (tests & TEST_STATUS) == 0 {
		log.Println("We don't support clients that don't support TEST_STATUS")
		return
	}
	testsToRun := []string{}
	runC2s := (tests & TEST_C2S) != 0
	runS2c := (tests & TEST_S2C) != 0

	if runC2s {
		testsToRun = append(testsToRun, strconv.Itoa(TEST_C2S))
	}
	if runS2c {
		testsToRun = append(testsToRun, strconv.Itoa(TEST_S2C))
	}

	sendNdtMessage(SrvQueue, "0", ws)
	sendNdtMessage(MsgLogin, "v5.0-NDTinGO", ws)
	sendNdtMessage(MsgLogin, strings.Join(testsToRun, " "), ws)

	var c2sRate, s2cRate float64
	if runC2s {
		c2sRate = manageC2sTest(ws)
		if c2sRate > 0 {
			testRate.WithLabelValues("c2s").Observe(c2sRate / 1000.0)
		}
	}
	if runS2c {
		s2cRate = manageS2cTest(ws)
		if s2cRate > 0 {
			testRate.WithLabelValues("s2c").Observe(s2cRate / 1000.0)
		}
	}

	sendNdtMessage(MsgResults, fmt.Sprintf("You uploaded at %.4f and downloaded at %.4f", c2sRate, s2cRate), ws)
	sendNdtMessage(MsgLogout, "", ws)
}

func DefaultHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(`
This is an NDT server.

It only works with Websockets and SSL.

You can monitor its status on port :9090/metrics.
`))
}

func main() {
	flag.Parse()

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		mux.Handle("/metrics", promhttp.Handler())
		log.Fatal(http.ListenAndServe(":9090", mux))
	}()

	http.HandleFunc("/", DefaultHandler)
	http.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
	http.Handle("/ndt_protocol",
		promhttp.InstrumentHandlerInFlight(currentTests,
			promhttp.InstrumentHandlerDuration(testDuration,
				http.HandlerFunc(NdtServer))))

	log.Println("About to listen on " + *NdtPort + ". Go to http://127.0.0.1:" + *NdtPort + "/")
	log.Fatal(http.ListenAndServeTLS(":"+*NdtPort, *certFile, *keyFile, nil))
}
