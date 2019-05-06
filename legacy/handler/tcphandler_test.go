package handler

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/m-lab/go/httpx"
	"github.com/m-lab/go/rtx"
)

func Count200(w http.ResponseWriter, r *http.Request) {

}

func TestNewTCP(t *testing.T) {
	// Set up the forwarding server
	success := 0
	h := &http.ServeMux{}
	h.HandleFunc("/test_url", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		success++
	})
	forwardSrv := &http.Server{
		Addr:    ":0",
		Handler: h,
	}
	rtx.Must(httpx.ListenAndServeAsync(forwardSrv), "Could not start server")
	_, err := http.Get("http://" + forwardSrv.Addr + "/test_url")
	rtx.Must(err, "Forwarding server could not respond to get")
	if success != 1 {
		t.Error("GET was unsuccessful")
	}

	// Set up the TCP handler
	tcpH := NewTCP(forwardSrv.Addr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rtx.Must(tcpH.ListenAndServe(ctx, ":0"), "Could not start tcp server")

	rh := tcpH.(*rawHandler)

	t.Run("Test that GET forwarding works", func(t *testing.T) {
		url := "http://" + rh.listener.Addr().String() + "/test_url"
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
		conn, err := net.Dial("tcp", rh.listener.Addr().String())
		rtx.Must(err, "Could not connect")
		rtx.Must(conn.Close(), "Could not close")
	})

	t.Run("Test that we can't listen and run twice on the same port", func(t *testing.T) {
		err := tcpH.ListenAndServe(ctx, rh.listener.Addr().String())
		if err == nil {
			t.Error("We should not have been able to start a second server")
		}
	})
}

func TestNewTCPBrokenForwarding(t *testing.T) {
	// Set up the TCP handler
	tcpH := NewTCP("127.0.0.1:1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rtx.Must(tcpH.ListenAndServe(ctx, ":0"), "Could not start tcp server")

	rh := tcpH.(*rawHandler)
	client := &http.Client{
		Timeout: 10 * time.Millisecond,
	}
	_, err := client.Get("http://" + rh.listener.Addr().String() + "/test_url")
	if err == nil {
		t.Error("This should have failed")
	}
}
