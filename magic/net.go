package magic

import (
	"crypto/tls"
	"log"
	"net"
	"os"
	"time"

	guuid "github.com/google/uuid"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/ndt-server/bbr"
	"github.com/m-lab/ndt-server/magic/iface"
	"github.com/m-lab/tcp-info/inetdiag"
	"github.com/m-lab/tcp-info/tcp"
)

// Listener is a TCPListener that is suitable for raw TCP servers, HTTP servers,
// and TLS HTTP servers. The Conn's returned by Listener.Accept mediate access
// to the underlying Conn file descriptor, allowing callers to perform meta
// operations on the connection, e.g. GetUUID, EnableBBR, ReadInfo.
type Listener struct {
	*net.TCPListener
	connfile iface.ConnFile
}

// NewListener creates a new Listener using the given net.TCPListener.
func NewListener(l *net.TCPListener) *Listener {
	return &Listener{
		TCPListener: l,
		connfile:    &iface.RealConnInfo{},
	}
}

// Conn is returned by Listener.Accept and provides mediated access to
// additional operations on the Conn file descriptor.
type Conn struct {
	net.Conn
	fp      *os.File
	netinfo iface.NetInfo
}

// Addr supports the net.Addr interface and allows mediated access to operations
// on the parent Conn.
//
// Why is this necessary? The Conn type accessible to application code is a
// function of the server protocol. In particular, the tls.Conn is created by a
// TLS Server, regardless of the underlying Listener type. Because a tls.Conn is
// a struct, not an interface, and because tls.Conn does not provide access to
// the underlying net.Conn, we still need a way to access the underlying Conn
// type in order to access and operate on the underlying connection file
// descriptor.
//
// So, to support all server protocols using the Listener, we "piggyback" on the
// net.Conn interface supported by tls.Conns. In particular, the LocalAddr and
// RemoteAddr methods return an Addr type, which includes a parent reference to
// the associated Conn.
//
// Because the Addr includes references to the parent Conn, all Addrs should be
// released before calling Conn.Close.
type Addr struct {
	net.Addr
	parentConn *Conn
}

// ConnInfo provides operations on a Conn's underlying file descriptor.
type ConnInfo interface {
	GetUUID() (string, error)
	EnableBBR() error
	ReadInfo() (inetdiag.BBRInfo, tcp.LinuxTCPInfo, error)
}

// Accept a connection, set 3min keepalive, and return a Conn that enables
// ConnInfo operations on the underlying net.Conn file descriptor.
func (ln *Listener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	fp, err := ln.connfile.TCPConnToFile(tc)
	if err != nil {
		tc.Close()
		return nil, err
	}
	mc := &Conn{
		Conn:    tc,
		fp:      fp,
		netinfo: &iface.RealConnInfo{},
	}
	return mc, nil
}

// Close the underlying net.Conn and dup'd file descriptor. Note: all net.Addr's
// returned by LocalAddr and RemoteAddr should be released before calling Close.
func (mc *Conn) Close() error {
	mc.fp.Close()
	return mc.Conn.Close()
}

// EnableBBR sets the BBR congestion control on the TCP connection, if supported
// by the kernel. If unsupported, EnableBBR has no effect.
func (mc *Conn) EnableBBR() error {
	return bbr.Enable(mc.fp)
}

// ReadInfo reads metadata about the TCP connections. If BBR was not enabled on
// the underlying connection, then ReadInfo will return an empty BBRInfo struct.
// If TCP info metrics cannot be read, an error is returned.
func (mc *Conn) ReadInfo() (inetdiag.BBRInfo, tcp.LinuxTCPInfo, error) {
	bbrinfo, err := mc.netinfo.GetBBRInfo(mc.fp)
	if err != nil {
		bbrinfo = inetdiag.BBRInfo{}
	}
	tcpInfo, err := mc.netinfo.GetTCPInfo(mc.fp)
	if err != nil {
		return inetdiag.BBRInfo{}, tcp.LinuxTCPInfo{}, err
	}
	return bbrinfo, *tcpInfo, nil
}

// GetUUID returns the connection's UUID.
func (mc *Conn) GetUUID() (string, error) {
	id, err := mc.netinfo.GetUUID(mc.fp)
	if err != nil {
		// Use UUID v1 as fallback when SO_COOKIE isn't supported by kernel
		fallbackUUID, err := guuid.NewUUID()
		// NOTE: this could only fail when `GetTime` fails from guuid package.
		rtx.Must(err, "unable to fallback to uuid")

		// NOTE: at this point, id is guaranteed to not be empty.
		id = fallbackUUID.String()
	}
	return id, nil
}

// LocalAddr returns an Addr supporting the net.Addr interface, which provides
// access to the parent Conn.
func (mc *Conn) LocalAddr() net.Addr {
	return &Addr{
		Addr:       mc.Conn.LocalAddr(),
		parentConn: mc,
	}
}

// RemoteAddr returns an Addr supporting the net.Addr interface, which provides
// access to the parent Conn.
func (mc *Conn) RemoteAddr() net.Addr {
	return &Addr{
		Addr:       mc.Conn.RemoteAddr(),
		parentConn: mc,
	}
}

// ToTCPAddr is a helper function for extracting the net.TCPAddr type from a
// net.Addr of various origins. ToTCPAddr returns nil if addr does not contain a
// *net.TCPAddr.
func ToTCPAddr(addr net.Addr) *net.TCPAddr {
	switch a := addr.(type) {
	case *Addr:
		return a.Addr.(*net.TCPAddr)
	case *net.TCPAddr:
		return a
	default:
		log.Printf("unsupported conn type: %T", a)
		return nil
	}
}

// ToConnInfo is a helper function for extracting the ConnInfo interface from
// the net.Conn of various origins. ToConnInfo panics if conn does not contain a
// type supporting ConnInfo.
func ToConnInfo(conn net.Conn) ConnInfo {
	switch c := conn.(type) {
	case *Conn:
		return c
	case *tls.Conn:
		return c.LocalAddr().(*Addr).parentConn
	default:
		log.Printf("unsupported conn type: %T", c)
		return nil
	}
}
