package main

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/m-lab/go/osx"

	"github.com/m-lab/go/rtx"

	pipe "gopkg.in/m-lab/pipe.v3"
)

// Get a bunch of open ports, and then close them. Hopefully the ports will
// remain open for the next few microseconds so that we can use them in unit
// tests.
func getOpenPorts(n int) []string {
	ports := []string{}
	for i := 0; i < n; i++ {
		ts := httptest.NewServer(http.NewServeMux())
		defer ts.Close()
		u, err := url.Parse(ts.URL)
		rtx.Must(err, "Could not parse url to local server:", ts.URL)
		ports = append(ports, ":"+u.Port())
	}
	return ports
}

func SetupMain() func() {
	cleanups := []func(){}

	// Create self-signed certs in a temp directory.
	dir, err := ioutil.TempDir("", "TestContextCancelsMain")
	rtx.Must(err, "Could not create tempdir")

	certFile := dir + "/cert.pem"
	keyFile := dir + "/key.pem"

	rtx.Must(
		pipe.Run(
			pipe.Script("Create private key and self-signed certificate",
				pipe.Exec("openssl", "genrsa", "-out", keyFile),
				pipe.Exec("openssl", "req", "-new", "-x509", "-key", keyFile, "-out",
					certFile, "-days", "2", "-subj",
					"/C=XX/ST=State/L=Locality/O=Org/OU=Unit/CN=Name/emailAddress=test@email.address"),
			),
		),
		"Failed to generate server key and certs")

	// Set up the command-line args via environment variables:
	ports := getOpenPorts(4)
	for _, ev := range []struct{ key, value string }{
		{"METRICS_PORT", ports[0]},
		{"NDT7_PORT", ports[1]},
		{"LEGACY_PORT", ports[2]},
		{"LEGACY_TLS_PORT", ports[3]},
		{"CERT", certFile},
		{"KEY", keyFile},
	} {
		cleanups = append(cleanups, osx.MustSetenv(ev.key, ev.value))
	}
	return func() {
		os.RemoveAll(dir)
		for _, f := range cleanups {
			f()
		}
	}
}

func Test_ContextCancelsMain(t *testing.T) {
	// Set up certs and the environment vars for the commandline.
	cleanup := SetupMain()
	defer cleanup()

	// Set up the global context for main()
	ctx, cancel = context.WithCancel(context.Background())

	// Run main, but cancel it very soon after starting.
	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()
	before := runtime.NumGoroutine()
	// If this doesn't run forever, then canceling the context causes main to exit.
	main()
	time.Sleep(100 * time.Millisecond)
	// Make sure main() doesn't leak goroutines.
	after := runtime.NumGoroutine()
	if before != after {
		t.Errorf("After running NumGoroutines changed: %d to %d", before, after)
	}
}

func Test_MainIntegrationTest(t *testing.T) {
	// Set up certs and the environment vars for the commandline.
	cleanup := SetupMain()
	defer cleanup()

	// Set up the global context for main()
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	// Get the ports but remove the leading ":"
	wsPort := os.Getenv("LEGACY_PORT")[1:]
	wssPort := os.Getenv("LEGACY_TLS_PORT")[1:]
	ndt7Port := os.Getenv("NDT7_PORT")[1:]

	tests := []struct {
		name string
		cmd  string
	}{
		// Test legacy clients
		{
			name: "Connect legacy WS",
			cmd: "node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wsPort + " --protocol=ws --tests=16",
		},
		{
			name: "Upload legacy WS",
			cmd: "node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wsPort + " --protocol=ws --tests=18",
		},
		{
			name: "Download legacy WS",
			cmd: "node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wsPort + " --protocol=ws --tests=20",
		},
		{
			name: "Upload & Download legacy WS",
			cmd: "node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wsPort + " --protocol=ws --tests=22",
		},
		/*{
			// Start both tests, but kill the client during the upload test.
			// This causes the server to wait for a test that never comes. After the
			// timeout, the server should have cleaned up all outstanding goroutines.
			name: "Upload & Download legacy WS with S2C Timeout",
			cmd: "node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wsPort +
				" --protocol=ws --abort-c2s-early --tests=22 & " +
				"sleep 25",
		},*/
		// Test WSS clients with the legacy protocol.
		{
			name: "Upload legacy WSS",
			cmd: "node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wssPort + " --protocol=wss --acceptinvalidcerts --tests=18",
		},
		{
			name: "Download legacy WSS",
			cmd: "node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wssPort + " --protocol=wss --acceptinvalidcerts --tests=20",
		},
		{
			name: "Upload & Download legacy WSS",
			cmd: "node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wssPort + " --protocol=wss --acceptinvalidcerts --tests=22",
		},
		/*{
			// Start both tests, but kill the client during the upload test.
			// This causes the server to wait for a test that never comes. After the
			// timeout, the server should have cleaned up all outstanding goroutines.
			name: "Upload & Download legacy WSS with S2C Timeout",
			cmd: "node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wssPort +
				" --protocol=wss --acceptinvalidcerts --abort-c2s-early --tests=22 & " +
				"sleep 25",
		},*/
		// Test NDT7 clients
		{
			name: "Test the ndt7 protocol",
			cmd:  "ndt-cloud-client -skip-tls-verify -port " + ndt7Port,
		},
	}

	go main()
	time.Sleep(1) // Give main a little time to grab all the ports and start listening.

	before := runtime.NumGoroutine()
	wg := sync.WaitGroup{}
	// Run every test in parallel (the server must handle parallel tests just fine)
	for _, testCmd := range tests {
		wg.Add(1)
		go func(name, cmd string) {
			defer wg.Done()
			stdout, stderr, err := pipe.DividedOutput(pipe.Script(name, pipe.System(cmd)))
			if err != nil {
				t.Errorf("ERROR Command: %s\nStdout: %s\nStderr: %s\n",
					cmd, string(stdout), string(stderr))
			}
		}(testCmd.name, testCmd.cmd)
	}
	wg.Wait()
	cancel()
	// wg.Wait() waits until wg.Done() has been called the right number of times.
	// But wg.Done() is called by a goroutine as it exits, so if we proceed
	// immediately we might spuriously measure that there are too many goroutines
	// just because the wg.Done() call caused an immediate resumption in the main
	// test thread before the goroutine exit had completed. time.Sleep() deals with
	// this race condition by giving all goroutines more than enough time to finish
	// exiting.
	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()
	if before != after {
		stack := make([]byte, 10000)
		runtime.Stack(stack, true)
		t.Errorf("After running NumGoroutines changed: %d to %d. Stacke: %s", before, after, string(stack))

	}
}
