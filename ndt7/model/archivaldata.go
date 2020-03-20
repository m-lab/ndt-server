package model

import (
	"time"

	"github.com/m-lab/ndt-server/metadata"
	"github.com/m-lab/tcp-info/inetdiag"
	"github.com/m-lab/tcp-info/tcp"
)

// ArchivalData saves all instantaneous measurements over the lifetime of a test.
type ArchivalData struct {
	UUID               string
	StartTime          time.Time
	EndTime            time.Time
	ServerMeasurements []Measurement
	ClientMeasurements []Measurement
	ClientMetadata     []metadata.NameValue `json:",omitempty"`
}

// The Measurement struct contains measurement results. This structure is
// meant to be serialised as JSON as sent as a textual message. This
// structure is specified in the ndt7 specification.
type Measurement struct {
	AppInfo        *AppInfo        `json:",omitempty"`
	ConnectionInfo *ConnectionInfo `json:",omitempty" bigquery:"-"`
	BBRInfo        *BBRInfo        `json:",omitempty"`
	TCPInfo        *TCPInfo        `json:",omitempty"`
}

// AppInfo contains an application level measurement. This structure is
// described in the ndt7 specification.
type AppInfo struct {
	NumBytes    int64
	ElapsedTime int64
}

// ConnectionInfo contains connection info. This structure is described
// in the ndt7 specification.
type ConnectionInfo struct {
	Client string
	Server string
	UUID   string `json:",omitempty"`
}

// The BBRInfo struct contains information measured using BBR. This structure is
// an extension to the ndt7 specification. Variables here have the same
// measurement unit that is used by the Linux kernel.
type BBRInfo struct {
	inetdiag.BBRInfo
	ElapsedTime int64
}

// The TCPInfo struct contains information measured using TCP_INFO. This
// structure is described in the ndt7 specification.
type TCPInfo struct {
	tcp.LinuxTCPInfo
	ElapsedTime int64
}
