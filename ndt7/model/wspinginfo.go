package model

// WSPingInfo contains an application level (websocket) ping measurement data.
// This structure is described in the ndt7 specification.
type WSPingInfo struct {
	ElapsedTime int64
	LastRTT     int64 // TCPInfo.RTT is smoothed RTT, LastRTT is just a sample.
	MinRTT      int64
}
