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

// Implementation note: the following are just stubs; all API documentation
// actually lives in `bbr/bbr_linux.go`.

func Enable(*net.TCPConn) error {
	log.Warn("TCP BBR not available on this platform")
	return ErrBBRNoSupport
}

func RegisterFd(*net.TCPConn) error {
	log.Warn("TCP BBR not available on this platform")
	return ErrBBRNoSupport
}

func ExtractFd(addr string) (int, error) {
	log.Warn("TCP BBR not available on this platform")
	return -1, ErrBBRNoSupport
}

func GetBandwidthAndRTT(fd int) (float64, float64, error) {
	return 0, 0, ErrBBRNoSupport
}
