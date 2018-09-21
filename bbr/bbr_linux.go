package bbr

// #cgo CFLAGS: -Wall -Wextra -Werror -std=c11 -Wno-unused-parameter
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

func getBandwidthAndRTT(fp *os.File) (float64, float64, error) {
	bw := C.double(0)
	rtt := C.double(0)
	// Note: Fd() returns uintptr but on Unix we can safely use int for sockets.
	rv := C.get_bbr_info(C.int(fp.Fd()), &bw, &rtt)
	if rv != 0 {
		return 0.0, 0.0, syscall.Errno(rv)
	}
	return float64(bw), float64(rtt), nil
}
