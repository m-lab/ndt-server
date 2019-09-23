package model

import "github.com/m-lab/tcp-info/tcp"

// The TCPInfo struct contains information measured using TCP_INFO. This
// structure is described in the ndt7 specification.
type TCPInfo struct {
	tcp.LinuxTCPInfo
	ElapsedTime   int64
}
