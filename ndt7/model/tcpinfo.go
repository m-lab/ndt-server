package model

import (
	"time"

	"github.com/m-lab/tcp-info/tcp"
)

// The TCPInfo struct contains information measured using TCP_INFO. This
// structure is described in the ndt7 specification.
type TCPInfo struct {
	tcp.LinuxTCPInfo
	ElapsedTime   int64
}

// NewTCPInfo creates an ndt7 model from the TCPInfo struct returned from the kernel.
func NewTCPInfo(i *tcp.LinuxTCPInfo, start time.Time) *TCPInfo {
	return &TCPInfo{
		LinuxTCPInfo: *i,
		ElapsedTime:   int64(time.Now().Sub(start) / time.Microsecond),
	}
}
