package model

// The Measurement struct contains measurement results. This structure is
// meant to be serialised as JSON as sent as a textual message. This
// structure is specified in the ndt7 specification.
type Measurement struct {
	AppInfo        *AppInfo        `json:",omitempty"`
	ConnectionInfo *ConnectionInfo `json:",omitempty" bigquery:"-"`
	BBRInfo        *BBRInfo        `json:",omitempty"`
	TCPInfo        *TCPInfo        `json:",omitempty"`
}
