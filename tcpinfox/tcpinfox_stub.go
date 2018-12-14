// +build !linux

package tcpinfox

import (
	"os"

	"github.com/m-lab/ndt-cloud/ndt7/model"
)

func getTCPInfo(*os.File) (model.TCPInfo, error) {
	return model.TCPInfo{}, ErrNoSupport
}
