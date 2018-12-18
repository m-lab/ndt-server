package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/m-lab/ndt-cloud/legacy"
	"github.com/m-lab/ndt-cloud/ndt7/spec"
	"github.com/m-lab/ndt-cloud/ndt7/server/download"

	pipe "gopkg.in/m-lab/pipe.v3"
)

func Test_NDTe2e_WSS(t *testing.T) {
	certFile := "cert.pem"
	keyFile := "key.pem"

	// Create key & self-signed certificate.
	err := pipe.Run(
		pipe.Script("Create private key and self-signed certificate",
			pipe.Exec("openssl", "genrsa", "-out", keyFile),
			pipe.Exec("openssl", "req", "-new", "-x509", "-key", keyFile, "-out",
				certFile, "-days", "2", "-subj",
				"/C=XX/ST=State/L=Locality/O=Org/OU=Unit/CN=Name/emailAddress=test@email.address"),
		),
	)
	if err != nil {
		t.Fatalf("Failed to generate server key and certs: %s", err)
	}

	// Start a test server using the NdtServer as the entry point.
	mux := http.NewServeMux()
	mux.Handle(spec.DownloadURLPath, download.Handler{})

	mux.Handle("/ndt_protocol",
		&legacy.Server{
			KeyFile:  keyFile,
			CertFile: certFile,
		})
	ts := httptest.NewTLSServer(mux)
	defer ts.Close()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	// TODO: add a multi-client test.
	// Run the unittest client using `node`.
	tests := []struct {
		name string
		cmd  string
	}{
		{
			name: "Upload",
			cmd: "node ./testdata/unittest_client.js --server=" + u.Hostname() +
				" --port=" + u.Port() + " --protocol=wss --acceptinvalidcerts --tests=18",
		},
		{
			name: "Download",
			cmd: "node ./testdata/unittest_client.js --server=" + u.Hostname() +
				" --port=" + u.Port() + " --protocol=wss --acceptinvalidcerts --tests=20",
		},
		{
			name: "Upload & Download",
			cmd: "node ./testdata/unittest_client.js --server=" + u.Hostname() +
				" --port=" + u.Port() + " --protocol=wss --acceptinvalidcerts --tests=22",
		},
		{
			// Start both tests, but kill the client during the upload test.
			// This causes the server to wait for a test that never comes. After the
			// timeout, the server should have cleaned up all outstanding goroutines.
			name: "Upload & Download with S2C Timeout",
			cmd: "node ./testdata/unittest_client.js --server=" + u.Hostname() +
				" --port=" + u.Port() +
				" --protocol=wss --acceptinvalidcerts --abort-c2s-early --tests=22 & " +
				"sleep 25",
		},
		{
			name: "Test the ndt7 protocol",
			cmd:  "ndt-cloud-client -skip-tls-verify -port " + u.Port(),
		},
	}

	for _, testCmd := range tests {
		before := runtime.NumGoroutine()
		stdout, stderr, err := pipe.DividedOutput(
			pipe.Script(testCmd.name, pipe.System(testCmd.cmd)))
		if err != nil {
			t.Errorf("ERROR Command: %s\nStdout: %s\nStderr: %s\n",
				testCmd, string(stdout), string(stderr))
		}
		after := runtime.NumGoroutine()
		if before != after {
			t.Errorf("After running %s NumGoroutines changed: %d to %d",
				testCmd.name, before, after)
		}
		t.Log(string(stdout))
	}
}

func Test_NDTe2e_WS(t *testing.T) {
	// Start a test server using the NdtServer as the entry point.
	mux := http.NewServeMux()
	mux.Handle(ndt7.DownloadURLPath, ndt7.DownloadHandler{})

	mux.Handle("/ndt_protocol", &legacy.Server{})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	// TODO: add a multi-client test.
	// Run the unittest client using `node`.
	tests := []struct {
		name string
		cmd  string
	}{
		{
			name: "Upload",
			cmd: "node ./testdata/unittest_client.js --server=" + u.Hostname() +
				" --port=" + u.Port() + " --protocol=ws --tests=18",
		},
		{
			name: "Download",
			cmd: "node ./testdata/unittest_client.js --server=" + u.Hostname() +
				" --port=" + u.Port() + " --protocol=ws --tests=20",
		},
		{
			name: "Upload & Download",
			cmd: "node ./testdata/unittest_client.js --server=" + u.Hostname() +
				" --port=" + u.Port() + " --protocol=ws --tests=22",
		},
		{
			// Start both tests, but kill the client during the upload test.
			// This causes the server to wait for a test that never comes. After the
			// timeout, the server should have cleaned up all outstanding goroutines.
			name: "Upload & Download with S2C Timeout",
			cmd: "node ./testdata/unittest_client.js --server=" + u.Hostname() +
				" --port=" + u.Port() +
				" --protocol=ws --abort-c2s-early --tests=22 & " +
				"sleep 25",
		},
	}

	before := runtime.NumGoroutine()
	wg := sync.WaitGroup{}
	for _, testCmd := range tests {
		wg.Add(1)
		go func(name, cmd string) {
			defer wg.Done()
			stdout, stderr, err := pipe.DividedOutput(
				pipe.Script(cmd, pipe.System(cmd)))
			if err != nil {
				t.Errorf("ERROR Command: %s\nStdout: %s\nStderr: %s\n",
					cmd, string(stdout), string(stderr))
			}
			t.Log(string(stdout))
		}(testCmd.name, testCmd.cmd)
	}
	wg.Wait()
	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()
	if before != after {
		t.Errorf("After running all ws e2e tests NumGoroutines changed: %d to %d", before, after)
	}
}
