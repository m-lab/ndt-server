package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/m-lab/ndt-cloud/ndt7"
	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-cloud/legacy"
	"github.com/m-lab/ndt-cloud/ndt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

func manageS2cTest(ws *websocket.Conn) (float64, error) {
	// Create a testResponder instance.
	testResponder := legacy.NewResponder("S2C", 20*time.Second, *fCertFile, *fKeyFile)

	// Create a TLS server for running the S2C test.
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/ndt_protocol",
		promhttp.InstrumentHandlerCounter(
			testCount.MustCurryWith(prometheus.Labels{"direction": "s2c"}),
			http.HandlerFunc(testResponder.S2CTestHandler)))
	err := testResponder.StartTLSAsync(serveMux)
	if err != nil {
		return 0, err
	}
	defer testResponder.Close()

	done := make(chan float64)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	go func() {
		s2cKbps, err := testResponder.S2CController(ws)
		if err != nil {
			cancel()
			log.Println("S2C: S2CController error:", err)
			return
		}
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
	upgrader := legacy.MakeNdtUpgrader([]string{"ndt"})
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

	http.Handle(ndt7.DownloadURLPath, ndt7.DownloadHandler{})

	http.HandleFunc("/", defaultHandler)
	http.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
	http.Handle("/ndt_protocol",
		promhttp.InstrumentHandlerInFlight(currentTests,
			promhttp.InstrumentHandlerDuration(testDuration,
				http.HandlerFunc(NdtServer))))

	log.Println("About to listen on " + *fNdtPort + ". Go to http://127.0.0.1:" + *fNdtPort + "/")
	log.Fatal(http.ListenAndServeTLS(":"+*fNdtPort, *fCertFile, *fKeyFile, nil))
}
