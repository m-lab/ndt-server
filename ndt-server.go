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
	"syscall"

	"github.com/m-lab/go/flagx"
	"github.com/m-lab/go/httpx"
	"github.com/m-lab/go/rtx"

	"github.com/m-lab/ndt-cloud/legacy"
	"github.com/m-lab/ndt-cloud/logging"
	"github.com/m-lab/ndt-cloud/ndt7/server/download"
	"github.com/m-lab/ndt-cloud/ndt7/spec"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Flags that can be passed in on the command line
	metricsPort = flag.String("metrics_port", ":9090", "The address and port to use for prometheus metrics")
	ndt7Port    = flag.String("ndt7_port", ":443", "The address and port to use for the ndt7 test")
	ndtPort     = flag.String("legacy_port", ":3001", "The address and port to use for the unencrypted legacy NDT test")
	ndtTLSPort  = flag.String("legacy_tls_port", ":3010", "The address and port to use for the legacy NDT test over TLS")
	certFile    = flag.String("cert", "", "The file with server certificates in PEM format.")
	keyFile     = flag.String("key", "", "The file with server key in PEM format.")

	// Metrcs for Prometheus
	currentTests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ndt_control_current",
			Help: "A gauge of requests currently being served by the NDT control handler.",
		},
		[]string{"type"})
	testDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "ndt_control_duration_seconds",
			Help: "A histogram of request latencies to the control channel.",
			// Durations will likely be tri-modal: early failures (fast),
			// completed single test (slower), completed dual tests (slowest) or timeouts.
			Buckets: []float64{.1, 1, 10, 10.5, 11, 11.5, 12, 20, 21, 22, 30, 60},
		},
		[]string{"type", "code"},
	)
	lameDuck = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "lame_duck_experiment",
		Help: "Indicates when the server is in lame duck",
	})

	// Context for the whole program.
	ctx, cancel = context.WithCancel(context.Background())
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
	defer close(c)
	signal.Notify(c, syscall.SIGTERM)

	// Wait until we receive a SIGTERM or the context is canceled.
	select {
	case <-c:
		fmt.Println("Received SIGTERM")
	case <-ctx.Done():
		fmt.Println("Canceled")
	}
	// Set lame duck status. This will remain set until exit.
	lameDuck.Set(1)
	// When we receive a second SIGTERM, cancel the context and shut everything
	// down. This should cause main() to exit cleanly.
	select {
	case <-c:
		fmt.Println("Received SIGTERM")
		cancel()
	case <-ctx.Done():
		fmt.Println("Canceled")
	}
}

func main() {
	flag.Parse()
	rtx.Must(flagx.ArgsFromEnv(flag.CommandLine), "Could not parse env args")
	// TODO: Decide if signal handling is the right approach here.
	go catchSigterm()

	// Prometheus with some extras.
	monitorMux := http.NewServeMux()
	monitorMux.HandleFunc("/debug/pprof/", pprof.Index)
	monitorMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	monitorMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	monitorMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	monitorMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	monitorMux.Handle("/metrics", promhttp.Handler())
	rtx.Must(
		httpx.ListenAndServeAsync(&http.Server{
			Addr:    *metricsPort,
			Handler: monitorMux,
		}),
		"Could not start metric server")

	// The legacy protocol serving WS-based tests.
	// TODO: Add protocol-sniffing support for non-WS tests.
	legacyLabel := prometheus.Labels{"type": "legacy_ws"}
	legacyMux := http.NewServeMux()
	legacyMux.HandleFunc("/", defaultHandler)
	legacyMux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
	legacyMux.Handle(
		"/ndt_protocol",
		promhttp.InstrumentHandlerInFlight(
			currentTests.With(legacyLabel),
			promhttp.InstrumentHandlerDuration(
				testDuration.MustCurryWith(legacyLabel),
				&legacy.BasicServer{TLS: false})))
	legacyServer := &http.Server{
		Addr:    *ndtPort,
		Handler: logging.MakeAccessLogHandler(legacyMux),
	}
	log.Println("About to listen for unencrypted legacy NDT tests on " + *ndtPort)
	rtx.Must(httpx.ListenAndServeAsync(legacyServer), "Could not start unencrypted legacy NDT server")
	defer legacyServer.Close()

	// The legacy protocol serving WSS-based tests.
	legacyTLSLabel := prometheus.Labels{"type": "legacy_wss"}
	legacyTLSMux := http.NewServeMux()
	legacyTLSMux.HandleFunc("/", defaultHandler)
	legacyTLSMux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
	legacyTLSConfig := legacy.BasicServer{
		CertFile: *certFile,
		KeyFile:  *keyFile,
		TLS:      true,
	}
	legacyTLSMux.Handle(
		"/ndt_protocol",
		promhttp.InstrumentHandlerInFlight(
			currentTests.With(legacyTLSLabel),
			promhttp.InstrumentHandlerDuration(
				testDuration.MustCurryWith(legacyTLSLabel),
				&legacyTLSConfig)))
	legacyTLSServer := &http.Server{
		Addr:    *ndtTLSPort,
		Handler: logging.MakeAccessLogHandler(legacyTLSMux),
	}
	log.Println("About to listen for legacy WSS tests on " + *ndtTLSPort)
	rtx.Must(httpx.ListenAndServeTLSAsync(legacyTLSServer, *certFile, *keyFile), "Could not start legacy WSS server")
	defer legacyTLSServer.Close()

	// The ndt7 listener serving up NDT7 tests, likely on standard ports.
	ndt7Label := prometheus.Labels{"type": "ndt7"}
	ndt7Mux := http.NewServeMux()
	ndt7Mux.HandleFunc("/", defaultHandler)
	ndt7Mux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
	ndt7Mux.Handle(
		spec.DownloadURLPath,
		promhttp.InstrumentHandlerInFlight(
			currentTests.With(ndt7Label),
			promhttp.InstrumentHandlerDuration(
				testDuration.MustCurryWith(ndt7Label),
				&download.Handler{})))
	ndt7Server := &http.Server{
		Addr:    *ndt7Port,
		Handler: logging.MakeAccessLogHandler(ndt7Mux),
	}
	log.Println("About to listen for ndt7 tests on " + *ndt7Port)
	rtx.Must(httpx.ListenAndServeTLSAsync(ndt7Server, *certFile, *keyFile), "Could not start ndt7 server")
	defer ndt7Server.Close()

	// Serve until the context is canceled.
	<-ctx.Done()
}
