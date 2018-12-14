package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/m-lab/ndt-cloud/legacy"
	"github.com/m-lab/ndt-cloud/legacy/tcplistener"
	"github.com/m-lab/ndt-cloud/logging"
	"github.com/m-lab/ndt-cloud/ndt7"
	ndt7download "github.com/m-lab/ndt-cloud/ndt7/server/download"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Flags that can be passed in on the command line
var (
	fNdt7Port    = flag.Int("ndt7-port", 443, "The port to use for the ndt7 test")
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
	lameDuck = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "lame_duck_experiment",
		Help: "Indicates when the server is in lame duck",
	})
)

func init() {
	prometheus.MustRegister(currentTests)
	prometheus.MustRegister(testDuration)
	prometheus.MustRegister(lameDuck)
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

	http.Handle(ndt7.DownloadURLPath, ndt7download.Handler{})

	http.HandleFunc("/", defaultHandler)
	http.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
	ndtServer := &legacy.Server{
		CertFile: *fCertFile,
		KeyFile:  *fKeyFile,
	}
	http.Handle("/ndt_protocol",
		promhttp.InstrumentHandlerInFlight(currentTests,
			promhttp.InstrumentHandlerDuration(testDuration, ndtServer)))

	// The following is listening on the standard NDT port and without BBR.
	go func() {
		log.Fatal(http.ListenAndServeTLS(":"+*fNdtPort, *fCertFile, *fKeyFile, nil))
	}()
	log.Println("About to listen on " + *fNdtPort + ". Go to http://127.0.0.1:" + *fNdtPort + "/")

	// This is the ndt7 listener on a standard port
	ln, err := net.ListenTCP("tcp", &net.TCPAddr{Port: *fNdt7Port})
	if err != nil {
		log.Fatal(err)
	}
	s := &http.Server{Handler: logging.MakeAccessLogHandler(http.DefaultServeMux)}
	log.Fatal(s.ServeTLS(tcplistener.RawListener{TCPListener: ln},
		*fCertFile, *fKeyFile))
}
