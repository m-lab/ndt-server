package tcpinfox

import (
	"os"
	"syscall"
	"unsafe"

	"github.com/m-lab/ndt-cloud/ndt7/model"
)

func getTCPInfo(fp *os.File) (model.TCPInfo, error) {
	// Note: Fd() returns uintptr but on Unix we can safely use int for sockets.
	tcpInfo := syscall.TCPInfo{}
	tcpInfoLen := uint32(unsafe.Sizeof(tcpInfo))
	_, _, err := syscall.Syscall6(
		uintptr(syscall.SYS_GETSOCKOPT),
		uintptr(int(fp.Fd())),
		uintptr(syscall.SOL_TCP),
		uintptr(syscall.TCP_INFO),
		uintptr(unsafe.Pointer(&tcpInfo)),
		uintptr(unsafe.Pointer(&tcpInfoLen)),
		uintptr(0))
	metrics := model.TCPInfo{}
	if err != 0 {
		return metrics, err
	}
	// TODO(bassosimone): map more metrics. For now we only maps the metrics
	// that are meaningful to a client to understand the context.
	metrics.SmoothedRTT = float64(tcpInfo.Rtt) / 1000.0 // to msec
	metrics.RTTVar = float64(tcpInfo.Rttvar) / 1000.0   // to msec
	return metrics, nil
}
