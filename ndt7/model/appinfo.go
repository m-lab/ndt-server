package model

// AppInfo contains an application level measurement.
type AppInfo struct {
	// NumBytes is the number of bytes transferred so far.
	NumBytes int64 `json:"num_bytes"`
}
