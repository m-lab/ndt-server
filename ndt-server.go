package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/websocket"

	"github.com/m-lab/go/prometheusx"

	"github.com/m-lab/go/flagx"
	"github.com/m-lab/go/httpx"
	"github.com/m-lab/go/rtx"

	legacyhandler "github.com/m-lab/ndt-server/legacy/handler"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/metrics"
	"github.com/m-lab/ndt-server/ndt7/handler"
	"github.com/m-lab/ndt-server/ndt7/listener"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/platformx"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Flags that can be passed in on the command line
	ndt7Port      = flag.String("ndt7_port", ":443", "The address and port to use for the ndt7 test")
	legacyPort    = flag.String("legacy_port", ":3001", "The address and port to use for the unencrypted legacy NDT test")
	legacyWsPort  = flag.String("legacy_ws_port", ":3002", "The address and port to use for the legacy NDT Ws test")
	legacyWssPort = flag.String("legacy_wss_port", ":3010", "The address and port to use for the legacy NDT WsS test")
	certFile      = flag.String("cert", "", "The file with server certificates in PEM format.")
	keyFile       = flag.String("key", "", "The file with server key in PEM format.")
	dataDir       = flag.String("datadir", "/var/spool/ndt", "The directory in which to write data files")

	// A metric to use to signal that the server is in lame duck mode.
	lameDuck = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lame_duck_experiment",
		Help: "Indicates when the server is in lame duck",
	})

	// Context for the whole program.
	ctx, cancel = context.WithCancel(context.Background())
)

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

func init() {
	log.SetFlags(log.LUTC | log.LstdFlags | log.Lshortfile)
}

func main() {
	flag.Parse()
	rtx.Must(flagx.ArgsFromEnv(flag.CommandLine), "Could not parse env args")
	// TODO: Decide if signal handling is the right approach here.
	go catchSigterm()

	promSrv := prometheusx.MustServeMetrics()
	defer promSrv.Close()

	platformx.WarnIfNotFullySupported()

	// The legacy protocol serving non-HTTP-based tests - forwards to Ws-based
	// server if the first three bytes are "GET".
	legacyServer := legacyhandler.NewTCP(*legacyWsPort)
	rtx.Must(
		legacyServer.ListenAndServeRawAsync(ctx, *legacyPort),
		"Could not start raw server")

	// The legacy protocol serving Ws-based tests. Most clients are hard-coded to
	// connect to the raw server, which will forward things along.
	legacyWsLabel := prometheus.Labels{"type": "legacy_ws"}
	legacyWsMux := http.NewServeMux()
	legacyWsMux.HandleFunc("/", defaultHandler)
	legacyWsMux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
	legacyWsMux.Handle(
		"/ndt_protocol",
		promhttp.InstrumentHandlerInFlight(
			metrics.CurrentTests.With(legacyWsLabel),
			promhttp.InstrumentHandlerDuration(
				metrics.TestDuration.MustCurryWith(legacyWsLabel),
				legacyhandler.NewWS())))
	legacyWsServer := &http.Server{
		Addr:    *legacyWsPort,
		Handler: logging.MakeAccessLogHandler(legacyWsMux),
	}
	log.Println("About to listen for unencrypted legacy NDT tests on " + *legacyWsPort)
	rtx.Must(httpx.ListenAndServeAsync(legacyWsServer), "Could not start unencrypted legacy NDT server")
	defer legacyWsServer.Close()

	// Only start TLS-based services if certs and keys are provided
	if *certFile != "" && *keyFile != "" {
		// The legacy protocol serving WsS-based tests.
		legacyWssLabel := prometheus.Labels{"type": "legacy_wss"}
		legacyWssMux := http.NewServeMux()
		legacyWssMux.HandleFunc("/", defaultHandler)
		legacyWssMux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
		legacyWssConfig := legacyhandler.NewWSS(*certFile, *keyFile)
		legacyWssMux.Handle(
			"/ndt_protocol",
			promhttp.InstrumentHandlerInFlight(
				metrics.CurrentTests.With(legacyWssLabel),
				promhttp.InstrumentHandlerDuration(
					metrics.TestDuration.MustCurryWith(legacyWssLabel),
					legacyWssConfig)))
		legacyWssServer := &http.Server{
			Addr:    *legacyWssPort,
			Handler: logging.MakeAccessLogHandler(legacyWssMux),
		}
		log.Println("About to listen for legacy WsS tests on " + *legacyWssPort)
		rtx.Must(httpx.ListenAndServeTLSAsync(legacyWssServer, *certFile, *keyFile), "Could not start legacy WsS server")
		defer legacyWssServer.Close()

		// The ndt7 listener serving up NDT7 tests, likely on standard ports.
		ndt7Label := prometheus.Labels{"type": "ndt7"}
		ndt7Mux := http.NewServeMux()
		ndt7Mux.HandleFunc("/", defaultHandler)
		ndt7Mux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
		ndt7Handler := &handler.Handler{
			DataDir: *dataDir,
			Upgrader: websocket.Upgrader{
				CheckOrigin: func(r *http.Request) bool {
					return true // Allow cross origin resource sharing
				},
			},
		}
		ndt7Mux.Handle(
			spec.DownloadURLPath,
			promhttp.InstrumentHandlerInFlight(
				metrics.CurrentTests.With(ndt7Label),
				promhttp.InstrumentHandlerDuration(
					metrics.TestDuration.MustCurryWith(ndt7Label),
					http.HandlerFunc(ndt7Handler.Download))))
		ndt7Mux.Handle(
			spec.UploadURLPath,
			promhttp.InstrumentHandlerInFlight(
				metrics.CurrentTests.With(ndt7Label),
				promhttp.InstrumentHandlerDuration(
					metrics.TestDuration.MustCurryWith(ndt7Label),
					http.HandlerFunc(ndt7Handler.Upload))))
		ndt7Server := &http.Server{
			Addr:    *ndt7Port,
			Handler: logging.MakeAccessLogHandler(ndt7Mux),
		}
		log.Println("About to listen for ndt7 tests on " + *ndt7Port)
		rtx.Must(listener.ListenAndServeTLSAsync(ndt7Server, *certFile, *keyFile), "Could not start ndt7 server")
		defer ndt7Server.Close()
	} else {
		log.Printf("Cert=%q and Key=%q means no TLS services will be started.\n", *certFile, *keyFile)
	}

	// Serve until the context is canceled.
	<-ctx.Done()
}
