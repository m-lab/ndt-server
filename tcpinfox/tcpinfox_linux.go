package tcpinfox

import (
	"os"
	"syscall"
	"unsafe"

	"github.com/m-lab/tcp-info/tcp"
)

func getTCPInfo(fp *os.File) (*tcp.LinuxTCPInfo, error) {
	tcpInfo := tcp.LinuxTCPInfo{}
	tcpInfoLen := uint32(unsafe.Sizeof(tcpInfo))
	rawConn, rawConnErr := fp.SyscallConn()
	if rawConnErr != nil {
		return &tcpInfo, rawConnErr
	}
	var err syscall.Errno
	rawConn.Control(func(fd uintptr) {
		_, _, err = syscall.Syscall6(
			uintptr(syscall.SYS_GETSOCKOPT),
			fd,
			uintptr(syscall.SOL_TCP),
			uintptr(syscall.TCP_INFO),
			uintptr(unsafe.Pointer(&tcpInfo)),
			uintptr(unsafe.Pointer(&tcpInfoLen)),
			uintptr(0))
	})
	if err != 0 {
		return &tcpInfo, err
	}
	return &tcpInfo, nil
}
