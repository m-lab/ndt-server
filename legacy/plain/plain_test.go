package plain_test

import (
	"context"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/m-lab/go/httpx"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/ndt-server/legacy/plain"
)

func TestNewPlainServer(t *testing.T) {
	d, err := ioutil.TempDir("", "TestNewPlainServer")
	rtx.Must(err, "Could not create tempdir")
	defer os.RemoveAll(d)
	// Set up the proxied server
	success := 0
	h := &http.ServeMux{}
	h.HandleFunc("/test_url", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		success++
	})
	wsSrv := &http.Server{
		Addr:    ":0",
		Handler: h,
	}
	rtx.Must(httpx.ListenAndServeAsync(wsSrv), "Could not start server")
	// Sanity check that the proxied server is up and running.
	_, err = http.Get("http://" + wsSrv.Addr + "/test_url")
	rtx.Must(err, "Proxied server could not respond to get")
	if success != 1 {
		t.Error("GET was unsuccessful")
	}

	// Set up the plain server
	tcpS := plain.NewServer(d, wsSrv.Addr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rtx.Must(tcpS.ListenAndServe(ctx, ":0"), "Could not start tcp server")

	t.Run("Test that GET forwarding works", func(t *testing.T) {
		url := "http://" + tcpS.Addr().String() + "/test_url"
		r, err := http.Get(url)
		if err != nil {
			t.Error("Could not get URL", url)
		}
		if r == nil || r.StatusCode != 200 {
			t.Errorf("Bad response: %v", r)
		}
		if success != 2 {
			t.Error("We should have had a second success")
		}
	})

	t.Run("Test that no data won't crash things", func(t *testing.T) {
		conn, err := net.Dial("tcp", tcpS.Addr().String())
		rtx.Must(err, "Could not connect")
		rtx.Must(conn.Close(), "Could not close")
	})

	t.Run("Test that we can't listen and run twice on the same port", func(t *testing.T) {
		err := tcpS.ListenAndServe(ctx, tcpS.Addr().String())
		if err == nil {
			t.Error("We should not have been able to start a second server")
		}
	})
}

func TestNewPlainServerBrokenForwarding(t *testing.T) {
	d, err := ioutil.TempDir("", "TestNewPlainServerBrokenForwarding")
	rtx.Must(err, "Could not create tempdir")
	defer os.RemoveAll(d)
	// Set up the plain server.
	tcpS := plain.NewServer(d, "127.0.0.1:1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rtx.Must(tcpS.ListenAndServe(ctx, ":0"), "Could not start tcp server")

	client := &http.Client{
		Timeout: 10 * time.Millisecond,
	}
	_, err = client.Get("http://" + tcpS.Addr().String() + "/test_url")
	if err == nil {
		t.Error("This should have failed")
	}
}
