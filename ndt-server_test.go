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
		{"NDT7_ADDR", ports[0]},
		{"NDT5_ADDR", ports[1]},
		{"NDT5_WS_ADDR", ports[2]},
		{"NDT5_WSS_ADDR", ports[3]},
		{"NDT7_ADDR_CLEARTEXT", ports[4]},
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
	ndt5Addr := os.Getenv("NDT5_ADDR")[1:]
	wsAddr := os.Getenv("NDT5_WS_ADDR")[1:]
	wssAddr := os.Getenv("NDT5_WSS_ADDR")[1:]
	ndt7Addr := os.Getenv("NDT7_ADDR")[1:]
	ndt7AddrCleartext := os.Getenv("NDT7_ADDR_CLEARTEXT")[1:]

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
		// NDT5 TLV-only clients.
		{
			// NOTE: we must disable the middle-box test in the ndt5 TLV client because it unconditionally expects
			// that test to run irrespective of what the server supports.
			name: "web100clt (ndt5 TLV)",
			cmd:  "timeout 45s /bin/web100clt-without-json-support --name localhost --port " + ndt5Addr + " --disablemid",
		},
		{
			name: "libndt-client - ndt5 NDT with JSON, download test",
			cmd:  "timeout 45s /bin/libndt-client localhost --port " + ndt5Addr + " --download",
		},
		{
			name: "libndt-client - ndt5 NDT with JSON, upload test",
			cmd:  "timeout 45s /bin/libndt-client localhost --port " + ndt5Addr + " --upload",
		},
		// Verify that ndt5 clients don't crash when we agree to only run a subset of the requested tests.
		{
			name: "Request all tests with web100clt (with JSON)",
			cmd:  "timeout 45s /bin/web100clt-with-json-support --name localhost --port " + ndt5Addr,
		},
		// The ndt5 client without JSON support looks like it DOES crash, although
		// the exact cause has not been investigated.
		// TODO(https://github.com/m-lab/ndt-server/issues/66) - make the following test case pass:
		// 	{
		// 		name: "Request all tests with web100clt (ndt5 TLV)",
		// 		cmd:  "timeout 45s /bin/web100clt-without-json-support --name localhost --port " + ndt5Addr,
		// 	},
		// Test libndt JSON clients
		{
			name: "libndt-client - ndt5 NDT with JSON, download test",
			cmd:  "timeout 45s /bin/libndt-client localhost --port " + ndt5Addr + " --json --download",
		},
		{
			name: "libndt-client - ndt5 NDT with JSON, upload test",
			cmd:  "timeout 45s /bin/libndt-client localhost --port " + ndt5Addr + " --json --upload",
		},
		{
			name: "libndt-client - ndt7, download test",
			cmd:  "timeout 45s /bin/libndt-client localhost --port " + ndt7Addr + " --ndt7 --download",
			// Ignore data because Travis does not support BBR.  Once Travis does support BBR, delete this.
			ignoreData: true,
		},
		// Test ndt5 raw JSON clients
		{
			name: "web100clt (with JSON), no MID or SFW",
			cmd:  "timeout 45s /bin/web100clt-with-json-support --name localhost --port " + ndt5Addr,
		},
		// Test ndt5 WS clients connected to the HTTP port
		{
			name: "Upload & Download ndt5 WS",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wsAddr + " --protocol=ws --tests=22",
		},
		{
			name: "Upload ndt5 WS",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wsAddr + " --protocol=ws --tests=18",
		},
		{
			name: "Download ndt5 WS",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wsAddr + " --protocol=ws --tests=20",
		},
		// Test ndt5 WS clients connecting to the raw port
		{
			name: "Connect ndt5 WS (upload and download) to RAW port",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + ndt5Addr + " --protocol=ws --tests=22",
		},
		{
			// Start both tests, but kill the client during the upload test.
			// This causes the server to wait for a test that never comes. After the
			// timeout, the server should have cleaned up all outstanding goroutines.
			name: "Upload & Download ndt5 WS with S2C Timeout",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wsAddr +
				" --protocol=ws --abort-c2s-early --tests=22 & " +
				"sleep 25",
		},
		// Test WSS clients with the ndt5 protocol.
		{
			name: "Upload ndt5 WSS",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wssAddr + " --protocol=wss --acceptinvalidcerts --tests=18",
		},
		{
			name: "Download ndt5 WSS",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wssAddr + " --protocol=wss --acceptinvalidcerts --tests=20",
		},
		{
			name: "Upload & Download ndt5 WSS",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wssAddr + " --protocol=wss --acceptinvalidcerts --tests=22",
		},
		{
			// Start both tests, but kill the client during the upload test.
			// This causes the server to wait for a test that never comes. After the
			// timeout, the server should have cleaned up all outstanding goroutines.
			name: "Upload & Download ndt5 WSS with S2C Timeout",
			cmd: "timeout 45s node ./testdata/unittest_client.js --server=localhost " +
				" --port=" + wssAddr +
				" --protocol=wss --acceptinvalidcerts --abort-c2s-early --tests=22 & " +
				"sleep 25",
		},
		// Test NDT7 clients
		{
			name: "Test the ndt7 protocol",
			cmd:  "timeout 45s ndt7-client -no-verify -server localhost:" + ndt7Addr,
			// Ignore data because Travis does not support BBR.  Once Travis does support BBR, delete this.
			ignoreData: true,
		},
		{
			name: "Test the ndt7 protocol in cleartext",
			cmd:  "timeout 45s ndt7-client -scheme ws -server localhost:" + ndt7AddrCleartext,
			// Ignore data because Travis does not support BBR.  Once Travis does support BBR, delete this.
			ignoreData: true,
		},
		// Measurement Kit client
		{
			name: "measurement_kit testing ndt5 protocol",
			cmd:  "timeout 45s measurement_kit --no-bouncer --no-collector --no-json --no-geoip ndt -p " + ndt5Addr + " localhost",
		},
	}

	go main()
	time.Sleep(1 * time.Second) // Give main a little time to grab all the ports and start listening.

	log.Printf(
		"ndt5 plain port: %s\nndt5 ws port: %s\nndt5 wss port: %s\nndt7 port: %s\n",
		ndt5Addr, wsAddr, wssAddr, ndt7Addr)

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
					t.Errorf("ERROR %s gave error %q (Command: %s)\nStdout: %s\nStderr: %s\n",
						tc.name, err, tc.cmd, string(stdout), string(stderr))
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
