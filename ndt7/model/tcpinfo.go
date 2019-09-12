package model

import (
	"time"

	"github.com/m-lab/tcp-info/tcp"
)

// The TCPInfo struct contains information measured using TCP_INFO. This
// structure is described in the ndt7 specification.
type TCPInfo struct {
	BusyTime      int64
	BytesAcked    int64
	BytesReceived int64
	BytesSent     int64
	BytesRetrans  int64
	ElapsedTime   int64
	MinRTT        int64
	RTT           int64
	RTTVar        int64
	RWndLimited   int64
	SndBufLimited int64
}

// NewTCPInfo creates an ndt7 model from the TCPInfo struct returned from the kernel.
func NewTCPInfo(i *tcp.LinuxTCPInfo, start time.Time) *TCPInfo {
	// TODO(bassosimone): This function here makes the code nonportable because
	// it won't compile on non Linux systems. This function should be inside
	// the tcpinfo package so that it becomes portable.
	return &TCPInfo{
		BusyTime:      i.BusyTime,
		BytesAcked:    i.BytesAcked,
		BytesReceived: i.BytesReceived,
		BytesSent:     i.BytesSent,
		BytesRetrans:  i.BytesRetrans,
		ElapsedTime:   int64(time.Now().Sub(start) / time.Microsecond),
		MinRTT:        int64(i.MinRTT),
		RTT:           int64(i.RTT),
		RTTVar:        int64(i.RTTVar),
		RWndLimited:   i.RWndLimited,
		SndBufLimited: i.SndBufLimited,
	}
}
