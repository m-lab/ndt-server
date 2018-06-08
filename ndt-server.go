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

func readMessage(ws *websocket.Conn, expectedType byte) []byte {
	_, buffer, err := ws.ReadMessage()
	if err != nil {
		log.Fatal(err)
	}
	if buffer[0] != expectedType {
		log.Fatal("Wrong message type. Wanted", expectedType, "got", buffer[0])
	}
	return buffer[3:]
}

// NdtJSONMessage holds the JSON messages we can receive from the server. We
// only support the subset of the NDT JSON protocol that has two fields: msg,
// and tests.
type NdtJSONMessage struct {
	msg   string
	tests string
}

func readJSONMessage(ws *websocket.Conn, expectedType byte) NdtJSONMessage {
	var message NdtJSONMessage
	var arbitraryMessage interface{}
	jsonString := readMessage(ws, expectedType)
	err := json.Unmarshal(jsonString, &arbitraryMessage)
	if err != nil {
		log.Fatal(err)
	}
	for k, v := range arbitraryMessage.(map[string]interface{}) {
		switch k {
		case "msg":
			message.msg = v.(string)
		case "tests":
			message.tests = v.(string)
		default:
			log.Fatal("Surprise JSON element:", k, v)
		}
	}
	return message
}

func sendPreformattedNdtMessage(msgType byte, message []byte, ws *websocket.Conn) {
	outbuff := make([]byte, 3+len(message))
	outbuff[0] = msgType
	outbuff[1] = byte((len(message) >> 8) & 0xFF)
	outbuff[2] = byte(len(message) & 0xFF)
	for i := range message {
		outbuff[i+3] = message[i]
	}
	err := ws.WriteMessage(websocket.BinaryMessage, outbuff)
	if err != nil {
		log.Fatal(err)
	}
}

func sendNdtMessage(msgType byte, msg []byte, ws *websocket.Conn) {
	message := []byte("{ \"msg\": \"" + string(msg) + "\" }")
	sendPreformattedNdtMessage(msgType, message, ws)
}

func makeNdtUpgrader(protocols []string) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:    819200,
		WriteBufferSize:   819200,
		Subprotocols:      protocols,
		EnableCompression: false,
		CheckOrigin: func(r *http.Request) bool {
			// TODO: make this check more appropriate -- added to get initial html5 widget to work.
			// log.Println("Origin:", r.Header.Get("Origin"))
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
	tr.response <- S2cReady
	totalBytes := float64(0)
	startTime := time.Now()
	endTime := startTime.Add(10 * time.Second)
	log.Println(tr.port, "S2C: Test starts at", time.Now().Format(time.RFC3339), "and ends in", (10 * time.Second))
	for time.Now().Before(endTime) {
		err := ws.WritePreparedMessage(messageToSend)
		if err != nil {
			log.Println(tr.port, "ERROR S2C: readmessage", err)
			return
		}
		totalBytes += float64(len(dataToSend))
	}
	megabitsPerSecond := float64(8) * totalBytes / float64(1000) / float64(time.Since(startTime)/time.Second)
	tr.response <- megabitsPerSecond
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
	totalBytes := float64(0)
	startTime := time.Now()
	endTime := startTime.Add(10 * time.Second)
	log.Println(tr.port, "C2S: Test starts at", time.Now().Format(time.RFC3339), "and ends in", (10 * time.Second))
	i := 0
	for time.Now().Before(endTime) {
		// if i%200 == 0 {
		// log.Println("C2S: ReadMessage at", time.Now().Format(time.RFC3339))
		// }
		_, buffer, err := ws.ReadMessage()
		if err != nil {
			log.Println(tr.port, "ERROR C2S: readmessage", err)
			return
		}
		totalBytes += float64(len(buffer))
		i++
	}
	megabitsPerSecond := float64(8) * totalBytes / float64(1000) / float64(time.Since(startTime)/time.Second)
	log.Println(tr.port, "C2S: Reporting rate:", megabitsPerSecond)
	tr.response <- megabitsPerSecond

	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	done := make(chan bool)

	// TODO: does this need to be 4s?
	// Drain the buffer for a while.
	go func() {
		endTime = time.Now().Add(4 * time.Second)
		for time.Now().Before(endTime) {
			//log.Println("reading second message")
			_, _, err := ws.ReadMessage()
			if err != nil {
				// log.Println("ERROR C2S: returning due to error in second read:", err)
				done <- true
				return
			}
			//log.Println("got second message")
		}
		done <- true
	}()
	ts := time.Now()
	log.Println(tr.port, "C2S: Waiting for reader or ticker")
	select {
	case <-done:
		// fmt.Println("Done!")
		break
	case <-ticker.C:
		//fmt.Println("Current time: ", t)
		break
	}
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
	s := http.Server{
		Addr:    ":0", // let OS randomly select port.
		Handler: serveMux,
	}
	defer s.Close()

	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		log.Println(ws.RemoteAddr(), "ERROR C2S: Failed to listen on:", s.Addr, err)
		return -1
	}
	defer ln.Close()
	c2sServerPort := ln.Addr().(*net.TCPAddr).Port
	go func() {
		log.Println(c2sServerPort, "C2S: About to listen for C2S on", ln.Addr())
		_ = s.ServeTLS(tcpKeepAliveListener{ln.(*net.TCPListener)}, *certFile, *keyFile)
	}()
	testResponder.port = c2sServerPort

	// Tell the client to go
	sendNdtMessage(TestPrepare, []byte(strconv.Itoa(c2sServerPort)), ws)
	c2sReady := <-testResponder.response
	if c2sReady != C2sReady {
		log.Println(c2sServerPort, "ERROR C2S: Bad value received on the c2s channel", c2sReady)
		return -1
	}
	sendNdtMessage(TestStart, []byte(""), ws)
	c2sRate := <-testResponder.response
	sendNdtMessage(TestMsg, []byte(fmt.Sprintf("%.4f", c2sRate)), ws)
	sendNdtMessage(TestFinalize, []byte(""), ws)
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
	s := http.Server{
		Addr:    ":0", // let OS randomly select port.
		Handler: serveMux,
	}
	defer s.Close()
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		log.Println("S2C: Failed to listen on:", s.Addr, err)
		return -1
	}
	defer ln.Close()
	s2cServerPort := ln.Addr().(*net.TCPAddr).Port
	testResponder.port = s2cServerPort
	go func() {
		log.Println(s2cServerPort, "S2C: About to listen for S2C on", s2cServerPort)
		_ = s.ServeTLS(tcpKeepAliveListener{ln.(*net.TCPListener)}, *certFile, *keyFile)
	}()

	// Tell the client to go
	sendNdtMessage(TestPrepare, []byte(strconv.Itoa(int(s2cServerPort))), ws)
	s2cReady := <-testResponder.response
	if s2cReady != S2cReady {
		log.Println(s2cServerPort, "S2C: Bad value received on the s2c channel", s2cReady)
		return -1
	}
	sendNdtMessage(TestStart, []byte(""), ws)
	s2cRate := <-testResponder.response
	sendPreformattedNdtMessage(TestMsg,
		[]byte(fmt.Sprintf("{ \"ThroughputValue\": %.4f, \"UnsentDataAmount\": 0, \"TotalSentByte\": %d}",
			s2cRate, int64(s2cRate*10*100/8))), ws)
	clientRateMsg := readJSONMessage(ws, TestMsg)
	log.Println(s2cServerPort, "S2C: The client sent us:", clientRateMsg.msg)
	requiredWeb100Vars := []string{"AckPktsIn", "CountRTT", "CongestionSignals", "CurRTO", "CurMSS",
		"DataBytesOut", "DupAcksIn", "MaxCwnd", "MaxRwinRcvd", "PktsOut", "PktsRetrans", "RcvWinScale",
		"Sndbuf", "SndLimTimeCwnd", "SndLimTimeRwin", "SndLimTimeSender", "SndWinScale", "SumRTT", "Timeouts",
		"MaxRTT", "MinRTT"}

	for _, web100Var := range requiredWeb100Vars {
		sendNdtMessage(TestMsg, []byte(web100Var+": 0"), ws)
	}
	sendNdtMessage(TestFinalize, []byte(""), ws)
	clientRate, err := strconv.ParseFloat(clientRateMsg.msg, 64)
	if err != nil {
		log.Println(s2cServerPort, "S2C: Bad client rate:", err)
		return -1
	}
	log.Println(s2cServerPort, "S2C: client rate:", clientRate, "vs", s2cRate)
	return s2cRate // clientRate
}

func runMetaTest(ws *websocket.Conn) {
	sendNdtMessage(TestPrepare, []byte(""), ws)
	sendNdtMessage(TestStart, []byte(""), ws)
	message := readJSONMessage(ws, TestMsg)
	for message.msg != "" {
		log.Println("Meta message: ", message)
		message = readJSONMessage(ws, TestMsg)
	}
	sendNdtMessage(TestFinalize, []byte(""), ws)
}

// The whole of the NDT server socket communication is run from this method.
// Returning will close the websocket connection, and should only be done after
// all tests are run.
func NdtServer(w http.ResponseWriter, r *http.Request) {
	upgrader := makeNdtUpgrader([]string{"ndt"})
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ERROR SERVER:", err)
		return
	}
	defer ws.Close()

	message := readJSONMessage(ws, MsgExtendedLogin)
	tests, err := strconv.ParseInt(message.tests, 10, 64)
	if (tests & TEST_STATUS) == 0 {
		log.Println("We don't support clients that don't support TEST_STATUS")
		return
	}
	tests_to_run := []string{}
	run_c2s := (tests & TEST_C2S) != 0
	run_s2c := (tests & TEST_S2C) != 0

	if run_c2s {
		tests_to_run = append(tests_to_run, strconv.Itoa(TEST_C2S))
	}
	if run_s2c {
		tests_to_run = append(tests_to_run, strconv.Itoa(TEST_S2C))
	}

	sendNdtMessage(SrvQueue, []byte("0"), ws)
	sendNdtMessage(MsgLogin, []byte("v5.0-NDTinGO"), ws)
	sendNdtMessage(MsgLogin, []byte(strings.Join(tests_to_run, " ")), ws)

	var c2sRate, s2cRate float64
	if run_c2s {
		c2sRate = manageC2sTest(ws)
		if c2sRate > 0 {
			testRate.WithLabelValues("c2s").Observe(c2sRate / 1000.0)
		}
	}
	if run_s2c {
		s2cRate = manageS2cTest(ws)
		if s2cRate > 0 {
			testRate.WithLabelValues("s2c").Observe(s2cRate / 1000.0)
		}
	}

	sendNdtMessage(MsgResults, []byte(fmt.Sprintf("You uploaded at %.4f and downloaded at %.4f", c2sRate, s2cRate)), ws)
	sendNdtMessage(MsgLogout, []byte(""), ws)
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
	err := http.ListenAndServeTLS(":"+*NdtPort, *certFile, *keyFile, nil)
	if err != nil {
		log.Fatal(err)
	}
}
