// +build !linux

package bbr

import (
	"os"

	"github.com/m-lab/ndt-server/ndt7/model"
)

func enableBBR(*os.File) error {
	return ErrNoSupport
}

func getMaxBandwidthAndMinRTT(*os.File) (model.BBRInfo, error) {
	return model.BBRInfo{}, ErrNoSupport
}
