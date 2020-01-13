package model

// WSInfo contains an application level (websocket) ping measurement data.
// It may be melded into AppInfo.
// FIXME: describe this structure is in the ndt7 specification.
type WSInfo struct {
    ElapsedTime int64
    LastRTT     int64 // TCPInfo.RTT is smoothed RTT, LastRTT is just a sample.
    MinRTT      int64
}
