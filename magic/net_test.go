package magic

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/m-lab/go/rtx"
	"github.com/m-lab/tcp-info/inetdiag"
	"github.com/m-lab/tcp-info/tcp"
)

func init() {
	log.SetFlags(log.LUTC | log.Lshortfile)
}

type errorCI struct{}

func (f *errorCI) TCPConnToFile(tc *net.TCPConn) (*os.File, error) {
	return nil, fmt.Errorf("fake file from conn error")
}

func dialAsyncUntilCanceled(t *testing.T, addr string) {
	go func() {
		// Because the socket already exists, Dial will block until Accept is
		// called below.
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Errorf("unexpected failure to dial local conn: %v", err)
			return
		}
		// Wait until primary test routine closes conn and returns.
		buf := make([]byte, 1)
		c.Read(buf)
		c.Close()
	}()
}

func TestListener_Accept(t *testing.T) {
	// Successful Accept.
	addr := &net.TCPAddr{}
	tcpl, err := net.ListenTCP("tcp", addr)
	rtx.Must(err, "failed to listen during unit test")
	ln := NewListener(tcpl)
	defer ln.Close()
	dialAsyncUntilCanceled(t, tcpl.Addr().String())

	got, err := ln.Accept()
	if err != nil {
		t.Errorf("Listener.Accept() unexpected error = %v", err)
		return
	}
	if _, ok := got.(*Conn); !ok {
		t.Errorf("Listener.Accept() wrong Conn type = %T, want *Conn", got)
	}

	// Accept error
	addr = &net.TCPAddr{}
	tcpl, err = net.ListenTCP("tcp", addr)
	rtx.Must(err, "failed to listen during unit test")
	ln = NewListener(tcpl)
	defer ln.Close()
	// Close listener so Accept fails.
	tcpl.Close()
	// NOTE: a client dialing is unnecessary for accept to fail after listener is closed.

	_, err = ln.Accept()
	if err == nil {
		t.Errorf("Listener.Accept() expected error, got %#v", got)
		return
	}

	// ConnInfo error
	addr = &net.TCPAddr{}
	tcpl, err = net.ListenTCP("tcp", addr)
	rtx.Must(err, "failed to listen during unit test")
	ln = NewListener(tcpl)
	defer ln.Close()
	dialAsyncUntilCanceled(t, tcpl.Addr().String())

	// Force accept to receive an error when reading fd from conn.
	ln.connfile = &errorCI{}

	got, err = ln.Accept()
	if err == nil {
		t.Errorf("Listener.Accept() expected error, got = %#v", got)
		return
	}
}

type errorNetInfo struct{}

func (e *errorNetInfo) GetUUID(fp *os.File) (string, error) {
	return "", fmt.Errorf("fake get uuid error")
}
func (e *errorNetInfo) GetBBRInfo(fp *os.File) (inetdiag.BBRInfo, error) {
	return inetdiag.BBRInfo{}, nil
}
func (e *errorNetInfo) GetTCPInfo(fp *os.File) (*tcp.LinuxTCPInfo, error) {
	return nil, fmt.Errorf("fake get tcpinfo error")
}

func TestConn(t *testing.T) {
	// Setup listener.
	laddr := &net.TCPAddr{}
	tcpl, err := net.ListenTCP("tcp", laddr)
	rtx.Must(err, "failed to listen during unit test")
	ln := NewListener(tcpl)
	defer ln.Close()

	client, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		// Because the socket already exists, Dial will block until Accept is
		// called below.
		c, err := net.Dial("tcp", tcpl.Addr().String())
		if err != nil {
			t.Fatalf("failed to dial local conn: %v", err)
		}
		// Wait until primary test routine closes conn and returns.
		<-client.Done()
		c.Close()
	}()

	conn, err := ln.Accept()
	if err != nil {
		t.Errorf("Conn.Accept() unexpected error = %v", err)
		return
	}
	defer conn.Close()

	addr := ToTCPAddr(conn.LocalAddr())
	if addr == nil {
		t.Errorf("ToTCPAddr(conn.LocalAddr()) returned nil addr")
	}
	addr = ToTCPAddr(conn.RemoteAddr())
	if addr == nil {
		t.Errorf("ToTCPAddr(conn.RemoteAddr()) returned nil addr")
	}

	ci := ToConnInfo(conn)
	ci.EnableBBR()
	id, err := ci.GetUUID()
	if err != nil || id == "" {
		t.Errorf("ConnInfo.GetUUID error: %#v, %q", err, id)
	}
	bi, ti, err := ci.ReadInfo()
	if err != nil {
		// TODO: make testing work on non-linux platforms.
		t.Errorf("ConnInfo.ReadInfo error: %#v, %#v %#v", err, bi, ti)
	}

	// Reset the netinfo value to always fail.
	c := conn.(*Conn)
	c.netinfo = &errorNetInfo{}
	id, err = ci.GetUUID()
	if err != nil || id == "" {
		t.Errorf("ConnInfo.GetUUID error, got %#v, %q", err, id)
	}

	// Read info with an error fr
	bi, ti, err = ci.ReadInfo()
	if err == nil {
		// TODO: make testing work on non-linux platforms.
		t.Errorf("ConnInfo.ReadInfo expected error, got nil: %#v %#v", bi, ti)
	}
}

func TestToTCPAddr(t *testing.T) {
	baseAddr := &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 1234,
	}
	tests := []struct {
		name    string
		addr    net.Addr
		wantNil bool
	}{
		{
			name: "success-Addr",
			addr: &Addr{
				Addr: baseAddr,
			},
		},
		{
			name: "success-TCPAddr",
			addr: baseAddr,
		},
		{
			name: "unsupported-returns-nil",
			addr: &net.UDPAddr{
				IP:   net.ParseIP("127.0.0.1"),
				Port: 1234,
			},
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToTCPAddr(tt.addr)
			if (got == nil) != tt.wantNil {
				t.Errorf("ToTCPAddr() wrong value; got %#v, wantNil %t", got, tt.wantNil)
			}
		})
	}
}

func TestToConnInfo(t *testing.T) {
	// NOTE: because we cannot synthetically create a tls.Conn that wraps a
	// magic.Conn, we must setup an httptest server with TLS enabled. While we
	// do that, we use it to validate the regular HTTP server magic.Conn as
	// well.

	fakeHTTPReply := "HTTP/1.0 200 OK\n\ntest"
	tests := []struct {
		name    string
		conn    net.Conn
		withTLS bool
	}{
		{
			name:    "success-Conn",
			withTLS: false,
		},
		{
			name:    "success-tls.Conn",
			withTLS: true,
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		hj, ok := rw.(http.Hijacker)
		if !ok {
			t.Fatalf("httptest Server does not support Hijacker interface")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("failed to hijack responsewriter")
		}
		defer conn.Close()
		// Write a fake reply for the client.
		conn.Write([]byte(fakeHTTPReply))

		// Extract the ConnInfo from the hijacked conn.
		got := ToConnInfo(conn)
		if got == nil {
			t.Errorf("ToConnInfo() failed to return ConnInfo from conn")
		}
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := httptest.NewUnstartedServer(mux)
			// Setup local listener using our Listener rather than the default.
			laddr := &net.TCPAddr{
				IP: net.ParseIP("127.0.0.1"),
			}
			tcpl, err := net.ListenTCP("tcp", laddr)
			rtx.Must(err, "failed to listen during unit test")
			// Use our listener in the httptest Server.
			s.Listener = NewListener(tcpl)
			// Start a plain or tls server.
			if tt.withTLS {
				s.StartTLS()
			} else {
				s.Start()
			}
			defer s.Close()

			// Use the server-provided client for TLS settings.
			c := s.Client()
			req, err := http.NewRequest(http.MethodGet, s.URL, nil)
			rtx.Must(err, "Failed to create request to %s", s.URL)
			// Run request to run conn test in handler.
			resp, err := c.Do(req)
			rtx.Must(err, "failed to GET %s", s.URL)
			b, err := ioutil.ReadAll(resp.Body)
			rtx.Must(err, "failed to read reply from %s", s.URL)

			if string(b) != "test" {
				t.Errorf("failed to receive reply from server")
			}
		})
	}

	// Finally, verify that unsupported net.Conn types return nil.
	got := ToConnInfo(&net.UDPConn{})
	if got != nil {
		t.Errorf("ToConnInfo() returned ConInfo for unsupported type: %#v", got)
	}
}
