// +build !linux

package bbr

import (
	"os"

	"github.com/m-lab/tcp-info/inetdiag"
)

func enableBBR(*os.File) error {
	return ErrNoSupport
}

func getMaxBandwidthAndMinRTT(*os.File) (inetdiag.BBRInfo, error) {
	return inetdiag.BBRInfo{}, ErrNoSupport
}
