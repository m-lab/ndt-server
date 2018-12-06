package ndt

import (
	"log"
	"net"
	"time"

	"github.com/m-lab/ndt-cloud/bbr"
	"github.com/m-lab/ndt-cloud/fdcache"
)

// RawListener is the place where we accept new TCP connections and
// set specific options on such connections. We unconditionally set the
// keepalive timeout for all connections, so that dead TCP connections
// (e.g. laptop closed amid a download) eventually go away. If the
// TryToEnableBBR setting is true, we additionally try to (1) enable
// BBR on the socket; (2) record the *os.File bound to a *net.TCPConn
// such that later we can collect BBR stats (see the bbr package for
// more info). As the name implies, TryToEnableBBR does its best to
// enable BBR but not succeding is also acceptable especially on systems
// where there is no support for BBR.
//
// Note: Adapted from net/http package.
type RawListener struct {
	*net.TCPListener
	TryToEnableBBR bool
}

// Accept accepts the TCP connection and then sets the connection's options.
func (ln RawListener) Accept() (net.Conn, error) {
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
	if ln.TryToEnableBBR {
		err = bbr.Enable(fp)
		if err != nil && err != bbr.ErrNoSupport {
			log.Printf("Cannot initialize BBR: %s", err.Error())
			// We need to close both because fp is a dup() of the original tc.
			fp.Close()
			tc.Close()
			return nil, err
		}
		if err == bbr.ErrNoSupport {
			// Keep going. There are also old Linux servers without BBR and servers
			// where the operating system is different from Linux.
		}
	}
	// Transfer ownership of |fp| to fdcache so that later we can retrieve
	// it from the generic net.Conn object bound to a websocket.Conn.
	fdcache.OwnFile(tc, fp)
	return tc, nil
}
