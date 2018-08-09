package netx

import (
	"net"
	"syscall"

	"github.com/apex/log"
)

func EnableBBR(tc *net.TCPConn) error {
	file, err := tc.File()
	if err != nil {
		log.WithError(err).Warn("Cannot obtain a File from a TCPConn")
		return err
	}
	fd := file.Fd()
	// Note: casting to int is safe because a socket is int on Unix
	err = syscall.SetsockoptString(int(fd), syscall.IPPROTO_TCP,
		syscall.TCP_CONGESTION, "bbr")
	if err != nil {
		log.WithError(err).Warn("SetsockoptString() failed")
		return err
	}
	log.Info("TCP BBR has been successfully enabled")
	return nil
}
