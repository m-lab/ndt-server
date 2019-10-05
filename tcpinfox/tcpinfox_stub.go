// +build !linux

package tcpinfox

import (
	"os"

	"github.com/m-lab/tcp-info/tcp"
)

func getTCPInfo(*os.File) (*tcp.LinuxTCPInfo, error) {
	return &tcp.LinuxTCPInfo{}, ErrNoSupport
}
