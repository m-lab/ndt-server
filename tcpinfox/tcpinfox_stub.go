// +build !linux

package tcpinfox

import (
	"os"
)

func getTCPInfo(*os.File) (TCPInfo, error) {
	return TCPInfo{}, ErrNoSupport
}
