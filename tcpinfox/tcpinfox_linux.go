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
	rawConn, err := fp.SyscallConn()
	if err != nil {
		return &tcpInfo, err
	}
	var syscallErr syscall.Errno
	err = rawConn.Control(func(fd uintptr) {
		_, _, syscallErr = syscall.Syscall6(
			uintptr(syscall.SYS_GETSOCKOPT),
			fd,
			uintptr(syscall.SOL_TCP),
			uintptr(syscall.TCP_INFO),
			uintptr(unsafe.Pointer(&tcpInfo)),
			uintptr(unsafe.Pointer(&tcpInfoLen)),
			uintptr(0))
	})
	if err != nil {
		return &tcpInfo, err
	}
	if syscallErr != 0 {
		return &tcpInfo, syscallErr
	}
	return &tcpInfo, nil
}
