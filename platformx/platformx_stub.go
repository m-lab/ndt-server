// +build !linux

package platformx

import (
	"github.com/m-lab/ndt-server/logging"
)

func maybeEmitWarning() {
	logging.Logger.Warn("This platform is not officially supported. It will work with reduced functionality.")
}
