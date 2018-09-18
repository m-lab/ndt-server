// +build !linux

// Package bbr contains TCP BBR related code.
package bbr

import (
	"errors"
	"net"

	"github.com/apex/log"
)

// ErrNoSupport is returned when BBR support is not compiled in.
var ErrNoSupport = errors.New("TCP BBR not available on this platform")

// Implementation note: the following are just stubs; all API documentation
// actually lives in `bbr/bbr_linux.go`.

func Enable(*net.TCPConn) error {
	log.Warn("TCP BBR not available on this platform")
	return ErrNoSupport
}

func RegisterFd(*net.TCPConn) error {
	log.Warn("TCP BBR not available on this platform")
	return ErrNoSupport
}

func ExtractFd(addr string) (int, error) {
	log.Warn("TCP BBR not available on this platform")
	return -1, ErrNoSupport
}

func GetBandwidthAndRTT(fd int) (float64, float64, error) {
	return 0, 0, ErrNoSupport
}
