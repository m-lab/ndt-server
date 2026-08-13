package bbr

import (
	"math"
	"os"
	"syscall"
	"unsafe"

	"github.com/m-lab/tcp-info/inetdiag"
	"golang.org/x/sys/unix"
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
	// The kernel's union tcp_cc_info is exactly as large as its largest
	// member, struct tcp_bbr_info (five 32 bit words), so a TCPBBRInfo
	// value can hold the TCP_CC_INFO output for any congestion control
	// algorithm. See include/uapi/linux/inet_diag.h.
	cci := unix.TCPBBRInfo{}
	size := uint32(unsafe.Sizeof(cci))
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
			uintptr(syscall.IPPROTO_TCP),
			uintptr(unix.TCP_CC_INFO),
			uintptr(unsafe.Pointer(&cci)),
			uintptr(unsafe.Pointer(&size)),
			uintptr(0))
	})
	if err != nil {
		return metrics, err
	}
	if syscallErr != 0 {
		// The system returns ENOSYS when it does not support BBR. In
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
	if size != uint32(unsafe.Sizeof(cci)) {
		return metrics, syscall.EINVAL
	}
	// Convert the values from the kernel provided units to the units that
	// we're going to use in ndt7. The units we use are the most common ones
	// in which people typically expects these variables.
	maxbw := uint64(cci.Bw_hi)<<32 | uint64(cci.Bw_lo)
	if maxbw > math.MaxInt64 {
		return metrics, syscall.EOVERFLOW
	}
	metrics.BW = int64(maxbw) // Java has no uint64
	metrics.MinRTT = cci.Min_rtt
	metrics.PacingGain = cci.Pacing_gain
	metrics.CwndGain = cci.Cwnd_gain
	return metrics, nil
}
