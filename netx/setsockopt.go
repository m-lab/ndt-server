// +build !linux

// Package netx extends the functionality of the net package. It contains, for
// example, code to turn on TCP BBR on a per-socket basis.
package netx

import (
	"net"

	"github.com/apex/log"
)

// EnableBBR taks in input a TCP connection and attempts to enable the BBR
// congestion control algorithm for that connection. Returns the error that
// occured. If BBR is not available on this platform, it does nothing.
func EnableBBR(*net.TCPConn) error {
	log.Warn("TCP BBR not available on this platform")
	return nil
}
