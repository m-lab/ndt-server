package ndt7test

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/m-lab/go/testingx"
	"github.com/m-lab/ndt-server/ndt7/handler"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/netx"
)

// NewNDT7Server creates a local httptest server capable of running an ndt7
// measurement in unittests.
func NewNDT7Server(t *testing.T) (*handler.Handler, *httptest.Server) {
	dir, err := ioutil.TempDir("", "ndt7test-*")
	testingx.Must(t, err, "failed to create temp dir")

	// TODO: add support for token verifiers.
	// TODO: add support for TLS server.
	ndt7Handler := &handler.Handler{DataDir: dir}
	ndt7Mux := http.NewServeMux()
	ndt7Mux.Handle(spec.DownloadURLPath, http.HandlerFunc(ndt7Handler.Download))
	ndt7Mux.Handle(spec.UploadURLPath, http.HandlerFunc(ndt7Handler.Upload))

	// Create unstarted so we can setup a custom netx.Listener.
	ts := httptest.NewUnstartedServer(ndt7Mux)
	listener, err := net.Listen("tcp", ":0")
	testingx.Must(t, err, "failed to allocate a listening tcp socket")
	addr := (listener.(*net.TCPListener)).Addr().(*net.TCPAddr)
	// Populate insecure port value with dynamic port.
	ndt7Handler.InsecurePort = fmt.Sprintf(":%d", addr.Port)
	ts.Listener = netx.NewListener(listener.(*net.TCPListener))
	// Now that the test server has our custom listener, start it.
	ts.Start()
	return ndt7Handler, ts
}
