package model

import "github.com/m-lab/tcp-info/tcp"

// The TCPInfo struct contains information measured using TCP_INFO.
type TCPInfo struct {
	// SmoothedRTT is the smoothed RTT in milliseconds.
	SmoothedRTT float64 `json:"smoothed_rtt"`

	// RTTVar is the RTT variance in milliseconds.
	RTTVar float64 `json:"rtt_var"`
}

// NewTCPInfo creates an ndt7 model from the TCPInfo struct returned from the kernel.
func NewTCPInfo(kernelTCPInfo *tcp.LinuxTCPInfo) *TCPInfo {
	return &TCPInfo{
		// TODO(bassosimone): map more metrics. For now we only maps the metrics
		// that are meaningful to a client to understand the context.
		SmoothedRTT: float64(kernelTCPInfo.RTT) / 1000.0,    // to msec
		RTTVar:      float64(kernelTCPInfo.RTTVar) / 1000.0, // to msec
	}
}
