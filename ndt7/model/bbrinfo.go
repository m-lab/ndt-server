// Package model contains the ndt7 data model
package model

// The BBRInfo struct contains information measured using BBR.
type BBRInfo struct {
	// MaxBandwidth is the max bandwidth measured by BBR in bits per second.
	MaxBandwidth int64 `json:"max_bandwidth"`

	// MinRTT is the min RTT measured by BBR in milliseconds.
	MinRTT float64 `json:"min_rtt"`
}
