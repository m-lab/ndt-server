package tcpinfox

import (
	"os"
	"syscall"
	"unsafe"

	"github.com/m-lab/tcp-info/tcp"
)

func getTCPInfo(fp *os.File) (*tcp.LinuxTCPInfo, error) {
	// Note: Fd() returns uintptr but on Unix we can safely use int for sockets.
	tcpInfo := tcp.LinuxTCPInfo{}
	tcpInfoLen := uint32(unsafe.Sizeof(tcpInfo))
	_, _, err := syscall.Syscall6(
		uintptr(syscall.SYS_GETSOCKOPT),
		uintptr(int(fp.Fd())),
		uintptr(syscall.SOL_TCP),
		uintptr(syscall.TCP_INFO),
		uintptr(unsafe.Pointer(&tcpInfo)),
		uintptr(unsafe.Pointer(&tcpInfoLen)),
		uintptr(0))
	if err != 0 {
		return &tcpInfo, err
	}
	return &tcpInfo, nil
}
