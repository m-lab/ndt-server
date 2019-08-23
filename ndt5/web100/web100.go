// Package web100 provides web100 variables (or a simulation thereof) to
// interested systems. When run on not-BBR it is polling-based, when run on BBR
// it only needs to measure once.
package web100

import "github.com/m-lab/tcp-info/tcp"

// Metrics holds web100 data. According to the NDT5 protocol, each of these
// metrics is required. That does not mean each is required to be non-zero, but
// it does mean that the field should be present in any response.
type Metrics struct {
	// Milliseconds
	MaxRTT, MinRTT, SumRTT, CurRTO, SndLimTimeCwnd, SndLimTimeRwin, SndLimTimeSender uint32

	// Counters
	DataBytesOut                                                           uint64
	DupAcksIn, PktsOut, PktsRetrans, Timeouts, CountRTT, CongestionSignals uint32
	AckPktsIn                                                              uint32 // Called SegsIn in tcp-kis.txt

	// Octets
	MaxCwnd, MaxRwinRcvd, CurMSS, Sndbuf uint32

	// Scaling factors
	RcvWinScale, SndWinScale int

	// Useful metrics that are not part of the required set.
	BytesPerSecond float64
	TCPInfo        tcp.LinuxTCPInfo
}
