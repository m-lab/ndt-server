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

	// Run the unittest client using `node`.
	tests := []string{
		// Upload
		"node ./testdata/unittest_client.js --server=" + u.Hostname() +
			" --port=" + u.Port() + " --protocol=wss --acceptinvalidcerts --tests=18",
		// Download
		"node ./testdata/unittest_client.js --server=" + u.Hostname() +
			" --port=" + u.Port() + " --protocol=wss --acceptinvalidcerts --tests=20",
		// Both
		"node ./testdata/unittest_client.js --server=" + u.Hostname() +
			" --port=" + u.Port() + " --protocol=wss --acceptinvalidcerts --tests=22",
	}

	for _, testCmd := range tests {
		err = pipe.Run(pipe.System(testCmd))
		if err != nil {
			t.Error(err)
		}
	}
}

// $ node --version
// v8.11.0
