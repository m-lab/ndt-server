package model

// AppInfo contains an application level measurement. This structure is
// described in the ndt7 specification.
type AppInfo struct {
	NumBytes    int64
	ElapsedTime int64
}
