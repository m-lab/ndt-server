package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"gopkg.in/m-lab/pipe.v3"
)

func Test_NDTe2e(t *testing.T) {
	*certFile = "cert.pem"
	*keyFile = "key.pem"

	// Create key & self-signed certificate.
	err := pipe.Run(
		pipe.Script("Create private key and self-signed certificate",
			pipe.Exec("openssl", "genrsa", "-out", "key.pem"),
			pipe.Exec("openssl", "req", "-new", "-x509", "-key", "key.pem", "-out",
				"cert.pem", "-days", "2", "-subj",
				"/C=XX/ST=State/L=Locality/O=Org/OU=Unit/CN=Name/emailAddress=test@email.address"),
		),
	)
	if err != nil {
		t.Fatalf("Failed to generate server key and certs: %s", err)
	}

	// Start a test server using the NdtServer as the entry point.
	mux := http.NewServeMux()
	mux.Handle("/ndt_protocol", http.HandlerFunc(NdtServer))
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
	}

	for _, testCmd := range tests {
		stdout, stderr, err := pipe.DividedOutput(
			pipe.Script(testCmd.name, pipe.System(testCmd.cmd)))
		if err != nil {
			t.Errorf("ERROR Command: %s\nStdout: %s\nStderr: %s\n",
				testCmd, string(stdout), string(stderr))
		}
		t.Log(string(stdout))
	}
}
