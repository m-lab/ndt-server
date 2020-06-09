// Package iface provides access to network connection operations via file
// descriptor. The implementation MUST be correct by inspection.
package iface

import (
	"net"
	"os"

	"github.com/m-lab/ndt-server/bbr"
	"github.com/m-lab/ndt-server/tcpinfox"
	"github.com/m-lab/tcp-info/inetdiag"
	"github.com/m-lab/tcp-info/tcp"
	"github.com/m-lab/uuid"
)

// ConnFile provides access to underlying network file.
type ConnFile interface {
	DupFile(tc *net.TCPConn) (*os.File, error)
}

// NetInfo provides access to network connection metadata.
type NetInfo interface {
	GetUUID(fp *os.File) (string, error)
	GetBBRInfo(fp *os.File) (inetdiag.BBRInfo, error)
	GetTCPInfo(fp *os.File) (*tcp.LinuxTCPInfo, error)
}

// RealConnInfo implements both the ConnFile and NetInfo interfaces.
type RealConnInfo struct{}

// DupFile returns the corresponding *os.File. Note that the
// returned *os.File is a dup() of the original, hence you now have ownership
// of two objects that you need to remember to defer Close() of.
func (f *RealConnInfo) DupFile(tc *net.TCPConn) (*os.File, error) {
	// Implementation note: according to a 2013 message on golang-nuts [1], the
	// code that follows is broken on Unix because calling File() makes the socket
	// blocking so causing Go to use more threads and, additionally, "timer wheel
	// inside net package never fires". However, an April, 19 2018 commit
	// on src/net/tcpsock.go apparently has removed such restriction and so now
	// (i.e. since go1.11beta1) it's safe to use the code below [2, 3].
	//
	// [1] https://grokbase.com/t/gg/golang-nuts/1349whs82r
	//
	// [2] https://github.com/golang/go/commit/60e3ebb9cba
	//
	// [3] https://github.com/golang/go/issues/24942
	//
	// For this reason, this code only works correctly with go >= 1.11.
	return tc.File()
}

// GetUUID returns a UUID for the given file pointer.
func (f *RealConnInfo) GetUUID(fp *os.File) (string, error) {
	return uuid.FromFile(fp)
}

// GetBBRInfo returns BBRInfo for the given file pointer.
func (f *RealConnInfo) GetBBRInfo(fp *os.File) (inetdiag.BBRInfo, error) {
	return bbr.GetBBRInfo(fp)
}

// GetTCPInfo returns TCPInfo for the given file pointer.
func (f *RealConnInfo) GetTCPInfo(fp *os.File) (*tcp.LinuxTCPInfo, error) {
	return tcpinfox.GetTCPInfo(fp)
}
