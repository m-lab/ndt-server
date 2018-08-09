package main

import (
	"flag"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/apex/log"
	"github.com/m-lab/ndt-cloud/ndt7"
)

var disableTLS = flag.Bool("disable-tls", false, "Whether to disable TLS")
var duration = flag.Int("duration", 10, "Desired duration")
var hostname = flag.String("hostname", "localhost", "Host to connect to")
var port = flag.String("port", "3001", "Port to connect to")
var skipTLSVerify = flag.Bool("skip-tls-verify", false, "Skip TLS verify")

func main() {
	flag.Parse()
	settings := ndt7.Settings{}
	settings.Hostname = *hostname
	settings.InsecureNoTLS = *disableTLS
	settings.InsecureSkipTLSVerify = *skipTLSVerify
	settings.Port = *port
	settings.Duration = *duration
	clnt := ndt7.NewClient(settings)
	ch := make(chan interface{}, 1)
	defer close(ch)
	sigs := make(chan os.Signal, 1)
	defer close(sigs)
	if runtime.GOOS != "windows" {
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs
			log.Warn("Got interrupt signal")
			ch <- false
			log.Warn("Delivered interrupt signal")
		}()
	}
	err := clnt.Download()
	if err != nil {
		log.WithError(err).Warn("clnt.Download() failed")
		os.Exit(1)
	}
}
