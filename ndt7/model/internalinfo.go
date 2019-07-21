package model

// InternalInfo contains fields that can change at any moment
// without notice. You are welcome to use them as long as it is
// clear that there's no API stability guarantee.
type InternalInfo struct {
	NumWritesDelta     int64
	SenderElapsedDelta float64
	WebSocketMsgSize   int64
}
