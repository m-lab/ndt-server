// Package bbr contains code required to read BBR variables of a net.Conn
// on which we're serving a WebSocket client. This code currently only
// works on Linux systems, as BBR is only available there.
package bbr

import (
	"errors"
	"os"

	"github.com/m-lab/ndt-server/ndt7/model"
)

// ErrNoSupport indicates that this system does not support BBR.
var ErrNoSupport = errors.New("TCP_CC_INFO not supported")

// Enable enables BBR on |fp|.
func Enable(fp *os.File) error {
	return enableBBR(fp)
}

// GetMaxBandwidthAndMinRTT obtains BBR info from |fp|.
func GetMaxBandwidthAndMinRTT(fp *os.File) (model.BBRInfo, error) {
	return getMaxBandwidthAndMinRTT(fp)
}
