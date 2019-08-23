// Package tcpinfox helps to gather TCP_INFO statistics.
package tcpinfox

import (
	"errors"
	"os"

	"github.com/m-lab/tcp-info/tcp"
)

// ErrNoSupport is returned on systems that do not support TCP_INFO.
var ErrNoSupport = errors.New("TCP_INFO not supported")

// GetTCPInfo measures TCP_INFO metrics using |fp| and returns them. In
// case of error, instead, an error is returned.
func GetTCPInfo(fp *os.File) (*tcp.LinuxTCPInfo, error) {
	return getTCPInfo(fp)
}
