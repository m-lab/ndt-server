// +build !linux

// Package bbr contains TCP BBR related code.
package bbr

import (
	"errors"
	"net"

	"github.com/apex/log"
)

// ErrBBRNoSupport is returned when BBR support is not compiled in.
var ErrBBRNoSupport = errors.New("TCP BBR not available on this platform")

// EnableBBR taks in input a TCP connection and attempts to enable the BBR
// congestion control algorithm for that connection. Returns nil in case
// of success, the error that occurred otherwise. Beware that the error might
// be ErrBBRNoSupport, in which case it's safe to continue, just knowing
// that you don't have BBR support on this platform.
func EnableBBR(*net.TCPConn) error {
	log.Warn("TCP BBR not available on this platform")
	return ErrBBRNoSupport
}

// RegisterBBRFd takes in input a TCP connection and maps its LocalAddr() to
// the corresponding file descriptor. This is used such that, later, it is
// possible to map back the corresponding connection (most likely a WebSockets
// connection wrapping a tls.Conn connection) to the file descriptor without
// using reflection, which might break with future versions of golang. If
// we have no BBR support, we return ErrBBRNoSupport.
func RegisterBBRFd(*net.TCPConn) error {
	log.Warn("TCP BBR not available on this platform")
	return ErrBBRNoSupport
}

// ExtractBBRFd checks whether there is a file descriptor corresponding to the
// provided address. If there is one, such file descriptor will be removed from
// the internal maps and returned. Otherwise ErrBBRNoFd is returned and the
// returned file descriptor will be set to -1 in this case. If there is no
// support for BBR, instead, ErrBBRNoSupport is returned.
func ExtractBBRFd(addr string) (int, error) {
	log.Warn("TCP BBR not available on this platform")
	return -1, ErrBBRNoSupport
}

// GetBBRInfo obtains BBR info from |fd|. The returned values are the
// max-bandwidth in bytes/s and the min-rtt in microseconds. The returned
// error is ErrBBRNoSupport if BBR is not supported on this platform.
func GetBBRInfo(fd int) (float64, float64, error) {
	return 0, 0, ErrBBRNoSupport
}
