package main

import (
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/m-lab/go/osx"
	"github.com/m-lab/go/prometheusx/promtest"
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

func countFiles(dir string) int {
	count := 0
	filepath.Walk(dir, func(_path string, info os.FileInfo, _err error) error {
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count
}

func setupMain() func() {
	cleanups := []func(){}

	// Create self-signed certs in a temp directory.
	dir, err := ioutil.TempDir("", "TestNdtServerMain")
	rtx.Must(err, "Could not create tempdir")

	certFile := "cert.pem"
	keyFile := "key.pem"

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
	ports := getOpenPorts(5)
	for _, ev := range []struct{ key, value string }{
		{"METRICS_PORT", ports[0]},
		{"NDT7_PORT", ports[1]},
		{"LEGACY_PORT", ports[2]},
		{"LEGACY_WS_PORT", ports[3]},
		{"LEGACY_WSS_PORT", ports[4]},
		{"CERT", certFile},
		{"KEY", keyFile},
		{"DATADIR", dir},
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
	cleanup := setupMain()
	defer cleanup()

	// Set up the global context for main()
	ctx, cancel = context.WithCancel(context.Background())
	before := runtime.NumGoroutine()

	// Run main, but cancel it very soon after starting.
	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()
	// If this doesn't run forever, then canceling the context causes main to exit.
	main()

	// A sleep has been added here to allow all completed goroutines to exit.
	time.Sleep(100 * time.Millisecond)

	// Make sure main() doesn't leak goroutines.
	after := runtime.NumGoroutine()
	if before != after {
		t.Errorf("After running NumGoroutines changed: %d to %d", before, after)
	}
}

func TestMetrics(t *testing.T) {
	promtest.LintMetrics(t)
}

func Test_MainIntegrationTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests take too long")
	}
	// Set up certs and the environment vars for the commandline.
	cleanup := setupMain()
	defer cleanup()

	// Set up the global context for main()
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	// Get the ports but remove the leading ":"
	legacyPort := os.Getenv("LEGACY_PORT")[1:]
	wsPort := os.Getenv("LEGACY_WS_PORT")[1:]
	wssPort := os.Getenv("LEGACY_WSS_PORT")[1:]
	ndt7Port := os.Getenv("NDT7_PORT")[1:]

	// Get the datadir
	dataDir := os.Getenv("DATADIR")

	type testcase struct {
		name string
		cmd  string
		// ignoreData's default value (false) will NOT ignore whether data is
		// produced. This is good, because it forces tests which ignore their output
		// data to explicitly specify this fact.
		ignoreData bool
	}
	tests := []testcase{
		// Before we can throw out the C NDT codebase:
		// TODO(https://github.com/m-lab/ndt-server/issues/65)
		//  /bin/web100clt-without-json-support --disablemid --disablesfw
		// TODO(https://github.com/m-lab/ndt-server/issues/66)
		//  /bin/web100clt-with-json-support    # No tests disabled.
		//  /bin/web100clt-without-json-support # No tests disabled.
		// Test legacy raw JSON clients
		{
			name: "Connect with web100clt (with JSON)",
			cmd:  "timeout 45s /bin/web100clt-with-json-support --name localhost --port " + legacyPort + " --disablemid --disablesfw",
		},
		// Test legacy WS clients connected to the HTTP port
		{
			name: "Upload & Download legacy WS",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wsPort + " --protocol=ws --tests=22",
		},
		{
			name: "Upload legacy WS",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wsPort + " --protocol=ws --tests=18",
		},
		{
			name: "Download legacy WS",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wsPort + " --protocol=ws --tests=20",
		},
		// Test legacy WS clients connecting to the raw port
		{
			name: "Connect legacy WS (upload and download) to RAW port",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + legacyPort + " --protocol=ws --tests=22",
		},
		{
			// Start both tests, but kill the client during the upload test.
			// This causes the server to wait for a test that never comes. After the
			// timeout, the server should have cleaned up all outstanding goroutines.
			name: "Upload & Download legacy WS with S2C Timeout",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wsPort +
				" --protocol=ws --abort-c2s-early --tests=22 & " +
				"sleep 25",
		},
		// Test WSS clients with the legacy protocol.
		{
			name: "Upload legacy WSS",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wssPort + " --protocol=wss --acceptinvalidcerts --tests=18",
		},
		{
			name: "Download legacy WSS",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wssPort + " --protocol=wss --acceptinvalidcerts --tests=20",
		},
		{
			name: "Upload & Download legacy WSS",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wssPort + " --protocol=wss --acceptinvalidcerts --tests=22",
		},
		{
			// Start both tests, but kill the client during the upload test.
			// This causes the server to wait for a test that never comes. After the
			// timeout, the server should have cleaned up all outstanding goroutines.
			name: "Upload & Download legacy WSS with S2C Timeout",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wssPort +
				" --protocol=wss --acceptinvalidcerts --abort-c2s-early --tests=22 & " +
				"sleep 25",
		},
		// Test NDT7 clients
		{
			name: "Test the ndt7 protocol",
			cmd:  "timeout 45s ndt-client -skip-tls-verify -port " + ndt7Port,
			// Ignore data because Travis does not support BBR.  Once Travis does support BBR, delete this.
			ignoreData: true,
		},
	}

	go main()
	time.Sleep(1 * time.Second) // Give main a little time to grab all the ports and start listening.

	log.Printf(
		"Legacy port: %s\n ws port: %s\nwss port: %s\nndt7 port: %s\n",
		legacyPort, wsPort, wssPort, ndt7Port)

	wg := sync.WaitGroup{}
	// Run every test in parallel (the server must handle parallel tests just fine)
	for _, c := range tests {
		wg.Add(1)
		func(tc testcase) {
			go t.Run(tc.name, func(t *testing.T) {
				defer wg.Done()
				preFileCount := countFiles(dataDir)
				stdout, stderr, err := pipe.DividedOutput(pipe.Script(tc.name, pipe.System(tc.cmd)))
				if err != nil {
					t.Errorf("ERROR %s (Command: %s)\nStdout: %s\nStderr: %s\n",
						tc.name, tc.cmd, string(stdout), string(stderr))
				}
				postFileCount := countFiles(dataDir)
				if !tc.ignoreData {
					// Verify that at least one data file was produced while the test ran.
					if postFileCount <= preFileCount {
						t.Error("No files produced. Before test:", preFileCount, "files. After test:", postFileCount, "files.")
					}
				}
				t.Logf("%s (command=%q) has completed successfully", tc.name, tc.cmd)
			})
		}(c)
	}
	wg.Wait()
}
