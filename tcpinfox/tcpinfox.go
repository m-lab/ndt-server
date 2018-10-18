// package tcpinfox helps to gather TCP_INFO statistics.
package tcpinfox

import (
	"errors"
	"os"
)

// The TCPInfo struct contains information measured using TCP_INFO.
type TCPInfo struct {
	// SmoothedRTT is the smoothed RTT in milliseconds.
	SmoothedRTT float64 `json:"smoothed_rtt"`

	// RTTVar is the RTT variance in milliseconds.
	RTTVar float64 `json:"rtt_var"`
}

// ErrNoSupport is returned on systems that do not support TCP_INFO.
var ErrNoSupport = errors.New("TCP_INFO not supported")

// GetTCPInfo measures TCP_INFO metrics using |fp| and returns them. In
// case of error, instead, an error is returned.
func GetTCPInfo(fp *os.File) (TCPInfo, error) {
	return getTCPInfo(fp)
}
