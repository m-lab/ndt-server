package bbr

// #cgo CFLAGS: -Wall -Wextra -Werror -std=c11 -Wno-unused-parameter
// #cgo LDFLAGS: -static
// #include "bbr_linux.h"
import "C"

import (
	"os"
	"syscall"
)

func enableBBR(fp *os.File) error {
	// Note: Fd() returns uintptr but on Unix we can safely use int for sockets.
	return syscall.SetsockoptString(int(fp.Fd()), syscall.IPPROTO_TCP,
		syscall.TCP_CONGESTION, "bbr")
}

func getMaxBandwidthAndMinRTT(fp *os.File) (float64, float64, error) {
	bw := C.double(0)
	rtt := C.double(0)
	// Note: Fd() returns uintptr but on Unix we can safely use int for sockets.
	rv := C.get_bbr_info(C.int(fp.Fd()), &bw, &rtt)
	if rv != 0 {
		// C.get_bbr_info returns ENOSYS when the system does not support BBR. In
		// such case let us map the error to ErrNoSupport, such that this Linux
		// system looks like any other system where BBR is not available. This way
		// the code for dealing with this error is not platform dependent.
		err := syscall.Errno(rv)
		if err == syscall.ENOSYS {
			return 0.0, 0.0, ErrNoSupport
		}
		return 0.0, 0.0, err
	}
	return float64(bw), float64(rtt), nil
}
