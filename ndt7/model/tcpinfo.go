package model

// The TCPInfo struct contains information measured using TCP_INFO.
type TCPInfo struct {
	// SmoothedRTT is the smoothed RTT in milliseconds.
	SmoothedRTT float64 `json:"smoothed_rtt"`

	// RTTVar is the RTT variance in milliseconds.
	RTTVar float64 `json:"rtt_var"`
}
