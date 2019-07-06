package tcplistener

import (
	"net"
	"time"

	"github.com/m-lab/ndt-server/fdcache"
)

// RawListener is the place where we accept new TCP connections and
// set specific options on such connections. We unconditionally set the
// keepalive timeout for all connections, so that dead TCP connections
// (e.g. laptop closed amid a download) eventually go away.
//
// Note: Adapted from net/http package.
type RawListener struct {
	*net.TCPListener
}

// Accept accepts the TCP connection and then sets the connection's options.
func (ln *RawListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	fp, err := fdcache.TCPConnToFile(tc)
	if err != nil {
		tc.Close()
		return nil, err
	}
	// Transfer ownership of |fp| to fdcache so that later we can retrieve
	// it from the generic net.Conn object bound to a websocket.Conn. We will
	// enable BBR at a later time and only if we really need it.
	//
	// Note: enabling BBR before performing the WebSocket handshake leaded
	// to the connection being stuck. See m-lab/ndt-server#37
	// <https://github.com/m-lab/ndt-server/issues/37>.
	fdcache.OwnFile(tc, fp)
	return tc, nil
}
