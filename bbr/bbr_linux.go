package bbr

// #include <linux/inet_diag.h>
// #include <netinet/ip.h>
// #include <netinet/tcp.h>
import "C"

import (
	"math"
	"os"
	"syscall"
	"unsafe"

	"github.com/m-lab/tcp-info/inetdiag"
)

func enableBBR(fp *os.File) error {
	rawconn, err := fp.SyscallConn()
	if err != nil {
		return err
	}
	var syscallErr error
	err = rawconn.Control(func(fd uintptr) {
		// Note: Fd() returns uintptr but on Unix we can safely use int for sockets.
		syscallErr = syscall.SetsockoptString(int(fd), syscall.IPPROTO_TCP, syscall.TCP_CONGESTION, "bbr")
	})
	if err != nil {
		return err
	}
	return syscallErr
}

func getMaxBandwidthAndMinRTT(fp *os.File) (inetdiag.BBRInfo, error) {
	cci := C.union_tcp_cc_info{}
	size := uint32(C.sizeof_union_tcp_cc_info)
	metrics := inetdiag.BBRInfo{}
	rawconn, rawConnErr := fp.SyscallConn()
	if rawConnErr != nil {
		return metrics, rawConnErr
	}
	var syscallErr syscall.Errno
	err := rawconn.Control(func(fd uintptr) {
		_, _, syscallErr = syscall.Syscall6(
			uintptr(syscall.SYS_GETSOCKOPT),
			fd,
			uintptr(C.IPPROTO_TCP),
			uintptr(C.TCP_CC_INFO),
			uintptr(unsafe.Pointer(&cci)),
			uintptr(unsafe.Pointer(&size)),
			uintptr(0))
	})
	if err != nil {
		return metrics, err
	}
	if syscallErr != 0 {
		// C.get_bbr_info returns ENOSYS when the system does not support BBR. In
		// such case let us map the error to ErrNoSupport, such that this Linux
		// system looks like any other system where BBR is not available. This way
		// the code for dealing with this error is not platform dependent.
		if syscallErr == syscall.ENOSYS {
			return metrics, ErrNoSupport
		}
		return metrics, syscallErr
	}
	// Apparently, tcp_bbr_info is the only congestion control data structure
	// to occupy five 32 bit words. Currently, in September 2018, the other two
	// data structures (i.e. Vegas and DCTCP) both occupy four 32 bit words.
	//
	// See include/uapi/linux/inet_diag.h in torvalds/linux@bbb6189d.
	if size != C.sizeof_struct_tcp_bbr_info {
		return metrics, syscall.EINVAL
	}
	bbrip := (*C.struct_tcp_bbr_info)(unsafe.Pointer(&cci[0]))
	// Convert the values from the kernel provided units to the units that
	// we're going to use in ndt7. The units we use are the most common ones
	// in which people typically expects these variables.
	maxbw := uint64(bbrip.bbr_bw_hi)<<32 | uint64(bbrip.bbr_bw_lo)
	if maxbw > math.MaxInt64 {
		return metrics, syscall.EOVERFLOW
	}
	metrics.BW = int64(maxbw) // Java has no uint64
	metrics.MinRTT = uint32(bbrip.bbr_min_rtt)
	metrics.PacingGain = uint32(bbrip.bbr_pacing_gain)
	metrics.CwndGain = uint32(bbrip.bbr_cwnd_gain)
	return metrics, nil
}
