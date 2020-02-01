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

	"github.com/m-lab/ndt-server/access"

	"github.com/m-lab/go/prometheusx"

	"github.com/m-lab/go/flagx"
	"github.com/m-lab/go/rtx"

	"github.com/m-lab/ndt-server/logging"
	ndt5handler "github.com/m-lab/ndt-server/ndt5/handler"
	"github.com/m-lab/ndt-server/ndt5/plain"
	"github.com/m-lab/ndt-server/ndt7/handler"
	"github.com/m-lab/ndt-server/ndt7/listener"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/platformx"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Flags that can be passed in on the command line
	ndt7Addr    = flag.String("ndt7_addr", ":443", "The address and port to use for the ndt7 test")
	ndt5Addr    = flag.String("ndt5_addr", ":3001", "The address and port to use for the unencrypted ndt5 test")
	ndt5WsAddr  = flag.String("ndt5_ws_addr", "127.0.0.1:3002", "The address and port to use for the ndt5 WS test")
	ndt5WssAddr = flag.String("ndt5_wss_addr", ":3010", "The address and port to use for the ndt5 WSS test")
	certFile    = flag.String("cert", "", "The file with server certificates in PEM format.")
	keyFile     = flag.String("key", "", "The file with server key in PEM format.")
	dataDir     = flag.String("datadir", "/var/spool/ndt", "The directory in which to write data files")
	maxConn     = flag.Int64("max-concurrent", 0, "The maximum number of concurrent client tests. Default unlimited.")

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

You can run an NDT5 test here:
   %s/static/widget.html (over http and websockets)
   %s/static/widget.html (over https and secure websockets)
or just by pointing an older NDT client at the addresses and ports serving those URLs.

NDT7 is recommended for all new clients. NDT5 is for existing clients
(including all versions before 5) that have not yet been ported to NDT7. The
version "NDT6" was skipped entirely. (IPv6 has been supported by NDT for many
years, and the name NDT6 risked confusion with the naming scheme used by
ping6 and the like).

You can monitor its status on port :9090/metrics.
`, *ndt7Addr, *ndt5Addr, *ndt5WssAddr)))
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

	cc := access.MaxController{
		Max: *maxConn,
	}

	// The ndt5 protocol serving non-HTTP-based tests - forwards to Ws-based
	// server if the first three bytes are "GET".
	ndt5Server := plain.NewServer(*dataDir+"/ndt5", *ndt5WsAddr)
	rtx.Must(
		ndt5Server.ListenAndServe(ctx, *ndt5Addr),
		"Could not start raw server")

	// The ndt5 protocol serving Ws-based tests. Most clients are hard-coded to
	// connect to the raw server, which will forward things along.
	ndt5WsMux := http.NewServeMux()
	ndt5WsMux.HandleFunc("/", defaultHandler)
	ndt5WsMux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
	ndt5WsMux.Handle("/ndt_protocol", ndt5handler.NewWS(*dataDir+"/ndt5"))
	ndt5WsServer := &http.Server{
		Addr:    *ndt5WsAddr,
		Handler: cc.Limit(logging.MakeAccessLogHandler(ndt5WsMux)),
	}
	log.Println("About to listen for unencrypted ndt5 NDT tests on " + *ndt5WsAddr)
	rtx.Must(listener.ListenAndServeAsync(ndt5WsServer), "Could not start unencrypted ndt5 NDT server")
	defer ndt5WsServer.Close()

	// Only start TLS-based services if certs and keys are provided
	if *certFile != "" && *keyFile != "" {
		// The ndt5 protocol serving WsS-based tests.
		ndt5WssMux := http.NewServeMux()
		ndt5WssMux.HandleFunc("/", defaultHandler)
		ndt5WssMux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
		ndt5WssMux.Handle("/ndt_protocol", ndt5handler.NewWSS(*dataDir+"/ndt5", *certFile, *keyFile))
		ndt5WssServer := &http.Server{
			Addr:    *ndt5WssAddr,
			Handler: cc.Limit(logging.MakeAccessLogHandler(ndt5WssMux)),
		}
		log.Println("About to listen for ndt5 WsS tests on " + *ndt5WssAddr)
		rtx.Must(listener.ListenAndServeTLSAsync(ndt5WssServer, *certFile, *keyFile), "Could not start ndt5 WsS server")
		defer ndt5WssServer.Close()

		// The ndt7 listener serving up NDT7 tests, likely on standard ports.
		ndt7Mux := http.NewServeMux()
		ndt7Mux.HandleFunc("/", defaultHandler)
		ndt7Mux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("html"))))
		ndt7Handler := &handler.Handler{
			DataDir: *dataDir,
		}
		ndt7Mux.Handle(spec.DownloadURLPath, http.HandlerFunc(ndt7Handler.Download))
		ndt7Mux.Handle(spec.UploadURLPath, http.HandlerFunc(ndt7Handler.Upload))
		ndt7Server := &http.Server{
			Addr:    *ndt7Addr,
			Handler: cc.Limit(logging.MakeAccessLogHandler(ndt7Mux)),
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
