// Package bbr contains code required to read BBR variables of a net.Conn
// on which we're serving a WebSocket client. This code currently only
// works on Linux systems, as BBR is only available there.
package bbr

import (
	"errors"
	"os"
)

// ErrNoSupport indicates that this system does not support BBR.
var ErrNoSupport = errors.New("No support for BBR")

// Enable enables BBR on |fp|.
func Enable(fp *os.File) error {
	return enableBBR(fp)
}

// GetMaxBandwidthAndMinRTT obtains BBR info from |fp|. The returned values are
// the max-bandwidth in bits per second and the min-rtt in milliseconds.
func GetMaxBandwidthAndMinRTT(fp *os.File) (float64, float64, error) {
	// Implementation note: for simplicity I have decided to use float64 here
	// rather than uint64, mainly because the proper C type to use AFAICT (and
	// I may be wrong here) changes between 32 and 64 bit. That is, it is not
	// clear to me how to use a 64 bit integer (which I what I would have used
	// by default) on a 32 bit system. So let's use float64.
	return getMaxBandwidthAndMinRTT(fp)
}
