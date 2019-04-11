package main

import (
	"crypto/tls"
	"flag"
	"os"

	"github.com/apex/log"
	"github.com/m-lab/ndt-server/cmd/ndt-client/client"
)

var hostname = flag.String("hostname", "localhost", "Host to connect to")
var port = flag.String("port", "3010", "Port to connect to")
var skipTLSVerify = flag.Bool("skip-tls-verify", false, "Skip TLS verify")

func main() {
	flag.Parse()
	clnt := client.Client{}
	clnt.URL.Scheme = "wss"
	clnt.URL.Host = *hostname + ":" + *port
	if *skipTLSVerify {
		config := tls.Config{InsecureSkipVerify: true}
		clnt.Dialer.TLSClientConfig = &config
	}
	if err := clnt.Download(); err != nil {
		log.WithError(err).Warn("clnt.Download() failed")
		os.Exit(1)
	}
	if err := clnt.Upload(); err != nil {
		log.WithError(err).Warn("clnt.Upload() failed")
		os.Exit(1)
	}
}
