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
	"github.com/m-lab/go/rtx"

	legacyhandler "github.com/m-lab/ndt-server/legacy/handler"
	"github.com/m-lab/ndt-server/legacy/plain"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/handler"
	"github.com/m-lab/ndt-server/ndt7/listener"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/platformx"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Flags that can be passed in on the command line
	ndt7Addr      = flag.String("ndt7_addr", ":443", "The address and port to use for the ndt7 test")
	legacyAddr    = flag.String("legacy_addr", ":3001", "The address and port to use for the unencrypted legacy NDT test")
	legacyWsAddr  = flag.String("legacy_ws_addr", "127.0.0.1:3002", "The address and port to use for the legacy NDT WS test")
	legacyWssAddr = flag.String("legacy_wss_addr", ":3010", "The address and port to use for the legacy NDT WsS test")
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
	w.Write([]byte(fmt.Sprintf(`
This is an NDT server.

You can run an NDT7 test (recommended) by going here:
   %s/static/ndt7.html

You can run a legacy test here: 
   %s/static/widget.html (over http and websockets)
   %s/static/widget.html (over https and secure websockets)

You can monitor its status on port :9090/metrics.
`, *ndt7Addr, *legacyAddr, *legacyWssAddr)))
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
	legacyServer := plain.NewServer(*dataDir+"/legacy", *legacyWsAddr)
	rtx.Must(
		legacyServer.ListenAndServe(ctx, *legacyAddr),
		"Could not start raw server")

	// The legacy protocol serving Ws-based tests. Most clients are hard-coded to
	// connect to the raw server, which will forward things along.
	legacyWsMux := http.NewServeMux()
	legacyWsMux.HandleFunc("/", defaultHandler)
	legacyWsMux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
	legacyWsMux.Handle("/ndt_protocol", legacyhandler.NewWS(*dataDir+"/legacy"))
	legacyWsServer := &http.Server{
		Addr:    *legacyWsAddr,
		Handler: logging.MakeAccessLogHandler(legacyWsMux),
	}
	log.Println("About to listen for unencrypted legacy NDT tests on " + *legacyWsAddr)
	rtx.Must(listener.ListenAndServeAsync(legacyWsServer), "Could not start unencrypted legacy NDT server")
	defer legacyWsServer.Close()

	// Only start TLS-based services if certs and keys are provided
	if *certFile != "" && *keyFile != "" {
		// The legacy protocol serving WsS-based tests.
		legacyWssMux := http.NewServeMux()
		legacyWssMux.HandleFunc("/", defaultHandler)
		legacyWssMux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
		legacyWssMux.Handle("/ndt_protocol", legacyhandler.NewWSS(*dataDir+"/legacy", *certFile, *keyFile))
		legacyWssServer := &http.Server{
			Addr:    *legacyWssAddr,
			Handler: logging.MakeAccessLogHandler(legacyWssMux),
		}
		log.Println("About to listen for legacy WsS tests on " + *legacyWssAddr)
		rtx.Must(listener.ListenAndServeTLSAsync(legacyWssServer, *certFile, *keyFile), "Could not start legacy WsS server")
		defer legacyWssServer.Close()

		// The ndt7 listener serving up NDT7 tests, likely on standard ports.
		ndt7Mux := http.NewServeMux()
		ndt7Mux.HandleFunc("/", defaultHandler)
		ndt7Mux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
		ndt7Handler := &handler.Handler{
			DataDir: *dataDir + "/ndt7",
			Upgrader: websocket.Upgrader{
				CheckOrigin: func(r *http.Request) bool {
					return true // Allow cross origin resource sharing
				},
			},
		}
		ndt7Mux.Handle(spec.DownloadURLPath, http.HandlerFunc(ndt7Handler.Download))
		ndt7Mux.Handle(spec.UploadURLPath, http.HandlerFunc(ndt7Handler.Upload))
		ndt7Server := &http.Server{
			Addr:    *ndt7Addr,
			Handler: logging.MakeAccessLogHandler(ndt7Mux),
		}
		log.Println("About to listen for ndt7 tests on " + *ndt7Addr)
		rtx.Must(listener.ListenAndServeTLSAsync(ndt7Server, *certFile, *keyFile), "Could not start ndt7 server")
		defer ndt7Server.Close()
	} else {
		log.Printf("Cert=%q and Key=%q means no TLS services will be started.\n", *certFile, *keyFile)
	}

	// Serve until the context is canceled.
	<-ctx.Done()
}
