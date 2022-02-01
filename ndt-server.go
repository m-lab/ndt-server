package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/m-lab/access/controller"
	"github.com/m-lab/access/token"
	"github.com/m-lab/go/flagx"
	"github.com/m-lab/go/prometheusx"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/metadata"
	ndt5handler "github.com/m-lab/ndt-server/ndt5/handler"
	"github.com/m-lab/ndt-server/ndt5/plain"
	"github.com/m-lab/ndt-server/ndt7/handler"
	"github.com/m-lab/ndt-server/ndt7/listener"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/platformx"
	"github.com/m-lab/ndt-server/version"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Flags that can be passed in on the command line
	ndt7Addr          = flag.String("ndt7_addr", ":443", "The address and port to use for the ndt7 test")
	ndt7AddrCleartext = flag.String("ndt7_addr_cleartext", ":80", "The address and port to use for the ndt7 cleartext test")
	ndt5Addr          = flag.String("ndt5_addr", ":3001", "The address and port to use for the unencrypted ndt5 test")
	ndt5WsAddr        = flag.String("ndt5_ws_addr", "127.0.0.1:3002", "The address and port to use for the ndt5 WS test")
	ndt5WssAddr       = flag.String("ndt5_wss_addr", ":3010", "The address and port to use for the ndt5 WSS test")
	certFile          = flag.String("cert", "", "The file with server certificates in PEM format.")
	keyFile           = flag.String("key", "", "The file with server key in PEM format.")
	tlsVersion        = flag.String("tls.version", "", "Minimum TLS version. Valid values: 1.2 or 1.3")
	dataDir           = flag.String("datadir", "/var/spool/ndt", "The directory in which to write data files")
	htmlDir           = flag.String("htmldir", "html", "The directory from which to serve static web content.")
	deploymentLabels  = flagx.KeyValue{}
	tokenVerifyKey    = flagx.FileBytesArray{}
	tokenRequired5    bool
	tokenRequired7    bool
	tokenMachine      string

	// A metric to use to signal that the server is in lame duck mode.
	lameDuck = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lame_duck_experiment",
		Help: "Indicates when the server is in lame duck",
	})

	// Context for the whole program.
	ctx, cancel = context.WithCancel(context.Background())
)

func init() {
	flag.Var(&tokenVerifyKey, "token.verify-key", "Public key for verifying access tokens")
	flag.BoolVar(&tokenRequired5, "ndt5.token.required", false, "Require access token in NDT5 requests")
	flag.BoolVar(&tokenRequired7, "ndt7.token.required", false, "Require access token in NDT7 requests")
	flag.StringVar(&tokenMachine, "token.machine", "", "Use given machine name to verify token claims")
	flag.Var(&deploymentLabels, "label", "Labels to identify the type of deployment.")
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

// httpServer creates a new *http.Server with explicit Read and Write timeouts.
func httpServer(addr string, handler http.Handler) *http.Server {
	tlsconf := &tls.Config{}
	switch *tlsVersion {
	case "1.3":
		tlsconf = &tls.Config{
			MinVersion: tls.VersionTLS13,
		}
	case "1.2":
		tlsconf = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}
	return &http.Server{
		Addr:      addr,
		Handler:   handler,
		TLSConfig: tlsconf,
		// NOTE: set absolute read and write timeouts for server connections.
		// This prevents clients, or middleboxes, from opening a connection and
		// holding it open indefinitely. This applies equally to TLS and non-TLS
		// servers.
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
	}
}

// parseDeploymentLabels() returns an array of key-value pairs of type
// []metadata.NameValue with the deployment label pairs passed in through
// the "label" flag.
func parseDeploymentLabels() []metadata.NameValue {
	labels := deploymentLabels.Get()
	serverMetadata := make([]metadata.NameValue, len(labels))
	index := 0

	for k, v := range labels {
		serverMetadata[index] = metadata.NameValue{
			Name:  k,
			Value: v,
		}
		index++

		// Add "-canary" to version, if applicable.
		if k == "deployment" && v == "canary" {
			version.Version += "-canary"
		}
	}

	return serverMetadata
}

func main() {
	flag.Parse()
	rtx.Must(flagx.ArgsFromEnv(flag.CommandLine), "Could not parse env args")

	serverMetadata := parseDeploymentLabels()

	// TODO: Decide if signal handling is the right approach here.
	go catchSigterm()

	promSrv := prometheusx.MustServeMetrics()
	defer promSrv.Close()

	platformx.WarnIfNotFullySupported()

	// Setup sequence of access control http.Handlers. NewVerifier errors are
	// not fatal as long as tokens are not required. This allows access tokens
	// to be optional for users who have no need for access tokens. An invalid
	// verifier is handled safely by Setup and only prints a warning when access
	// token verification is disabled.
	v, err := token.NewVerifier(tokenVerifyKey.Get()...)
	if (tokenRequired5 || tokenRequired7) && err != nil {
		rtx.Must(err, "Failed to load verifier for when tokens are required")
	}
	// NDT5 uses a raw server, which requires tx5. NDT7 is HTTP only.
	ac5, tx5 := controller.Setup(ctx, v, tokenRequired5, tokenMachine)
	ac7, _ := controller.Setup(ctx, v, tokenRequired7, tokenMachine)

	// The ndt5 protocol serving non-HTTP-based tests - forwards to Ws-based
	// server if the first three bytes are "GET".
	ndt5Server := plain.NewServer(*dataDir+"/ndt5", *ndt5WsAddr, serverMetadata)
	rtx.Must(
		ndt5Server.ListenAndServe(ctx, *ndt5Addr, tx5),
		"Could not start raw server")

	// The ndt5 protocol serving Ws-based tests. Most clients are hard-coded to
	// connect to the raw server, which will forward things along.
	ndt5WsMux := http.NewServeMux()
	ndt5WsMux.Handle("/", http.FileServer(http.Dir(*htmlDir)))
	ndt5WsMux.Handle("/ndt_protocol", ndt5handler.NewWS(*dataDir+"/ndt5", serverMetadata))
	controller.AllowPathLabel("/ndt_protocol")
	ndt5WsServer := httpServer(
		*ndt5WsAddr,
		// NOTE: do not use `ac.Then()` to prevent 'double jeopardy' for
		// forwarded clients when txcontroller is enabled.
		logging.MakeAccessLogHandler(ndt5WsMux),
	)
	log.Println("About to listen for unencrypted ndt5 NDT tests on " + *ndt5WsAddr)
	rtx.Must(listener.ListenAndServeAsync(ndt5WsServer), "Could not start unencrypted ndt5 NDT server")
	defer ndt5WsServer.Close()

	// The ndt7 listener serving up NDT7 tests, likely on standard ports.
	ndt7Mux := http.NewServeMux()
	ndt7Mux.Handle("/", http.FileServer(http.Dir(*htmlDir)))
	ndt7Handler := &handler.Handler{
		DataDir:        *dataDir,
		SecurePort:     *ndt7Addr,
		InsecurePort:   *ndt7AddrCleartext,
		ServerMetadata: serverMetadata,
	}
	ndt7Mux.Handle(spec.DownloadURLPath, http.HandlerFunc(ndt7Handler.Download))
	ndt7Mux.Handle(spec.UploadURLPath, http.HandlerFunc(ndt7Handler.Upload))
	controller.AllowPathLabel(spec.DownloadURLPath)
	controller.AllowPathLabel(spec.UploadURLPath)
	ndt7ServerCleartext := httpServer(
		*ndt7AddrCleartext,
		ac7.Then(logging.MakeAccessLogHandler(ndt7Mux)),
	)
	log.Println("About to listen for ndt7 cleartext tests on " + *ndt7AddrCleartext)
	rtx.Must(listener.ListenAndServeAsync(ndt7ServerCleartext), "Could not start ndt7 cleartext server")
	defer ndt7ServerCleartext.Close()

	// Only start TLS-based services if certs and keys are provided
	if *certFile != "" && *keyFile != "" {
		// The ndt5 protocol serving WsS-based tests.
		ndt5WssMux := http.NewServeMux()
		ndt5WssMux.Handle("/", http.FileServer(http.Dir(*htmlDir)))
		ndt5WssMux.Handle("/ndt_protocol", ndt5handler.NewWSS(*dataDir+"/ndt5", *certFile, *keyFile, serverMetadata))
		ndt5WssServer := httpServer(
			*ndt5WssAddr,
			ac5.Then(logging.MakeAccessLogHandler(ndt5WssMux)),
		)
		log.Println("About to listen for ndt5 WsS tests on " + *ndt5WssAddr)
		rtx.Must(listener.ListenAndServeTLSAsync(ndt5WssServer, *certFile, *keyFile), "Could not start ndt5 WsS server")
		defer ndt5WssServer.Close()

		// The ndt7 listener serving up WSS based tests
		ndt7Server := httpServer(
			*ndt7Addr,
			ac7.Then(logging.MakeAccessLogHandler(ndt7Mux)),
		)
		log.Println("About to listen for ndt7 tests on " + *ndt7Addr)
		rtx.Must(listener.ListenAndServeTLSAsync(ndt7Server, *certFile, *keyFile), "Could not start ndt7 server")
		defer ndt7Server.Close()
	} else {
		log.Printf("Cert=%q and Key=%q means no TLS services will be started.\n", *certFile, *keyFile)
	}

	// Serve until the context is canceled.
	<-ctx.Done()
}
