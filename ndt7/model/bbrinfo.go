// Package model contains the ndt7 data model
package model

// The BBRInfo struct contains information measured using BBR. This structure
// is an extension to the ndt7 specification. Variables here have the same
// measurement unit that is used by the Linux kernel.
type BBRInfo struct {
	ElapsedTime  int64
	MaxBandwidth int64
	MinRTT       int64
}
