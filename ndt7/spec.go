// Package ndt7 contains a non-backwards compatible redesign of the NDT
// network performance measurement protocol. The complete specification of
// the protocol is available at
// https://github.com/m-lab/ndt-cloud/blob/master/spec/ndt7.md.
package ndt7

// DownloadURLPath selects the download subtest.
const DownloadURLPath = "/ndt/v7/download"

// UploadURLPath selects the upload subtest.
const UploadURLPath = "/ndt/v7/upload"

// SecWebSocketProtocol is the WebSocket subprotocol used by ndt7.
const SecWebSocketProtocol = "net.measurementlab.ndt.v7"

// MinMaxMessageSize is the minimum value of the maximum message size
// that an implementation MAY want to configure. Messages smaller than this
// threshold MUST always be accepted by an implementation.
const MinMaxMessageSize = 1 << 17

// The BBRInfo struct contains information measured using BBR.
type BBRInfo struct {
	// MaxBandwidth is the max bandwidth measured by BBR in bits per second.
	MaxBandwidth float64 `json:"max_bandwidth"`

	// MinRTT is the min RTT measured by BBR in milliseconds.
	MinRTT float64 `json:"min_rtt"`
}

// The Measurement struct contains measurement results. This structure is
// meant to be serialised as JSON as sent on a textual message.
type Measurement struct {
	// Elapsed is the number of seconds elapsed since the beginning.
	Elapsed float64 `json:"elapsed"`

	// NumBytes is the number of bytes transferred since the beginning.
	NumBytes float64 `json:"num_bytes"`

	// BBRInfo is the data measured using TCP BBR instrumentation.
	BBRInfo *BBRInfo `json:"bbr_info,omitempty"`

	// Padding contains an optional random [A-Za-z]+ string that MAY be
	// added by a server to send larger measurement message, so that such
	// messages could be used directly to generate network load.
	Padding string `json:"padding,omitempty"`
}
