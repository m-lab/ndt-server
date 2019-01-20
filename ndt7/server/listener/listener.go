// Package httpx provides generic functions which extend the capabilities of
// the http package.
//
// The code here eliminates an annoying race condition in net/http that prevents
// you from knowing when it is safe to connect to the server socket. For the
// functions in this package, the listening socket is fully estabished when the
// function returns, and it is safe to run an HTTP GET immediately.
package httpx

import (
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

var logFatalf = log.Fatalf

// The code here is adapted from https://golang.org/src/net/http/server.go?s=85391:85432#L2742

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}

func serve(server *http.Server, listener net.Listener) {
	err := server.Serve(listener)
	if err != http.ErrServerClosed {
		logFatalf("Error, server %v closed with unexpected error %v", server, err)
	}
}

// ListenAndServeAsync starts an http server. The server will run until
// Shutdown() or Close() is called, but this function will return once the
// listening socket is established.  This means that when this function
// returns, the server is immediately available for an http GET to be run
// against it.
//
// Returns a non-nil error if the listening socket can't be established. Logs a
// fatal error if the server dies for a reason besides ErrServerClosed. If the
// server.Addr is set to :0, then after this function returns server.Addr will
// contain the address and port which this server is listening on.
func ListenAndServeAsync(server *http.Server) error {
	// Start listening synchronously.
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	if strings.HasSuffix(server.Addr, ":0") {
		// Allow :0 to select a random port, and then update the server with the
		// selected port and address.  This is very useful for unit tests.
		server.Addr = listener.Addr().String()
	}
	// Serve asynchronously.
	go serve(server, tcpKeepAliveListener{listener.(*net.TCPListener)})
	return nil
}

func serveTLS(server *http.Server, listener net.Listener, certFile, keyFile string) {
	err := server.ServeTLS(listener, certFile, keyFile)
	if err != http.ErrServerClosed {
		logFatalf("Error, server %v closed with unexpected error %v", server, err)
	}
}

// ListenAndServeTLSAsync starts an https server. The server will run until
// Shutdown() or Close() is called, but this function will return once the
// listening socket is established.  This means that when this function
// returns, the server is immediately available for an https GET to be run
// against it.
//
// Returns a non-nil error if the listening socket can't be established. Logs a
// fatal error if the server dies for a reason besides ErrServerClosed.
func ListenAndServeTLSAsync(server *http.Server, certFile, keyFile string) error {
	// Start listening synchronously.
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}

	// Unlike ListenAndServeAsync we don't update the server's Addr when the
	// server.Addr ends with :0, because the resulting URL may or may not be
	// GET-able. In ipv6-only contexts it could be, for example, "[::]:3232", and
	// that URL can't be used for TLS because TLS needs a name or an explicit IP
	// and [::] doesn't qualify. It is unclear what the right thing to do is in
	// this situation, because names and IPs and TLS are suffciently complicated
	// that no one thing is the right thing in all situations, so we affirmatively
	// do nothing in an attempt to avoid making a bad situation worse.

	// Serve asynchronously.
	go serveTLS(server, tcpKeepAliveListener{listener.(*net.TCPListener)}, certFile, keyFile)
	return nil
}
