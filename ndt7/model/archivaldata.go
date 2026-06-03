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
	// ClientMetadata contains entries from the client's query string and,
	// when present, integration claims (int_id, key_id) from the verified
	// access token.
	ClientMetadata []metadata.NameValue `json:",omitempty"`
	ServerMetadata     []metadata.NameValue `json:",omitempty"`
}

// The Measurement struct contains measurement results. This structure is
// meant to be serialised as JSON as sent as a textual message. This
// structure is specified in the ndt7 specification.
//
// NOTE: the declaration order of these fields determines the BigQuery column
// order produced by bigquery.InferSchema (see cmd/generate-schemas) and must
// match the legacy ndt7 table so the autoloaded and parser tables can be
// UNION'd without corruption (see m-lab/etl-schema ndt7_union, issue #195).
// New fields MUST be appended at the END.
type Measurement struct {
	AppInfo        *AppInfo        `json:",omitempty"`
	BBRInfo        *BBRInfo        `json:",omitempty"`
	TCPInfo        *TCPInfo        `json:",omitempty"`
	ConnectionInfo *ConnectionInfo `json:",omitempty"`
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
	Client    string
	Server    string
	UUID      string    `json:",omitempty"`
	StartTime time.Time `json:",omitempty"`
}

// The BBRInfo struct contains information measured using BBR. This structure is
// an extension to the ndt7 specification. Variables here have the same
// measurement unit that is used by the Linux kernel.
//
// NOTE: the fields of inetdiag.BBRInfo are enumerated explicitly (rather than
// embedded) so that the BigQuery column order is controlled here and matches
// the legacy ndt7 table. New fields MUST be appended at the END. Populate with
// NewBBRInfo.
type BBRInfo struct {
	BW         int64  `csv:"BBR.BW"`         // Max-filtered BW (app throughput) estimate in bytes/second
	MinRTT     uint32 `csv:"BBR.MinRTT"`     // Min-filtered RTT in uSec
	PacingGain uint32 `csv:"BBR.PacingGain"` // Pacing gain shifted left 8 bits
	CwndGain   uint32 `csv:"BBR.CwndGain"`   // Cwnd gain shifted left 8 bits

	ElapsedTime int64
}

// NewBBRInfo builds a BBRInfo from an inetdiag.BBRInfo sampled from the kernel.
func NewBBRInfo(bbr inetdiag.BBRInfo, elapsed int64) *BBRInfo {
	return &BBRInfo{
		BW:          bbr.BW,
		MinRTT:      bbr.MinRTT,
		PacingGain:  bbr.PacingGain,
		CwndGain:    bbr.CwndGain,
		ElapsedTime: elapsed,
	}
}

// The TCPInfo struct contains information measured using TCP_INFO. This
// structure is described in the ndt7 specification.
//
// NOTE: the fields of tcp.LinuxTCPInfo are enumerated explicitly (rather than
// embedded) so that the BigQuery column order is controlled here. The order
// below reproduces the legacy ndt7 BigQuery table exactly: ElapsedTime sits
// between ReordSeen and RcvOooPack/SndWnd for historical reasons (those two
// kernel fields were added to the legacy table after ElapsedTime already
// existed). Matching this order lets the autoloaded tables be UNION'd with the
// legacy parser table without corrupting nested values (issue #195).
//
// IMPORTANT: any NEW field (added to tcp.LinuxTCPInfo upstream, or added here)
// MUST be appended at the END, after SndWnd. Never insert in the middle.
// Populate with NewTCPInfo.
type TCPInfo struct {
	State       uint8 `csv:"TCP.State"`
	CAState     uint8 `csv:"TCP.CAState"`
	Retransmits uint8 `csv:"TCP.Retransmits"`
	Probes      uint8 `csv:"TCP.Probes"`
	Backoff     uint8 `csv:"TCP.Backoff"`
	Options     uint8 `csv:"TCP.Options"`
	WScale      uint8 `csv:"TCP.WScale"`
	AppLimited  uint8 `csv:"TCP.AppLimited"`

	RTO    uint32 `csv:"TCP.RTO"`
	ATO    uint32 `csv:"TCP.ATO"`
	SndMSS uint32 `csv:"TCP.SndMSS"`
	RcvMSS uint32 `csv:"TCP.RcvMSS"`

	Unacked uint32 `csv:"TCP.Unacked"`
	Sacked  uint32 `csv:"TCP.Sacked"`
	Lost    uint32 `csv:"TCP.Lost"`
	Retrans uint32 `csv:"TCP.Retrans"`
	Fackets uint32 `csv:"TCP.Fackets"`

	LastDataSent uint32 `csv:"TCP.LastDataSent"`
	LastAckSent  uint32 `csv:"TCP.LastAckSent"`
	LastDataRecv uint32 `csv:"TCP.LastDataRecv"`
	LastAckRecv  uint32 `csv:"TCP.LastDataRecv"`

	PMTU        uint32 `csv:"TCP.PMTU"`
	RcvSsThresh uint32 `csv:"TCP.RcvSsThresh"`
	RTT         uint32 `csv:"TCP.RTT"`
	RTTVar      uint32 `csv:"TCP.RTTVar"`
	SndSsThresh uint32 `csv:"TCP.SndSsThresh"`
	SndCwnd     uint32 `csv:"TCP.SndCwnd"`
	AdvMSS      uint32 `csv:"TCP.AdvMSS"`
	Reordering  uint32 `csv:"TCP.Reordering"`

	RcvRTT   uint32 `csv:"TCP.RcvRTT"`
	RcvSpace uint32 `csv:"TCP.RcvSpace"`

	TotalRetrans uint32 `csv:"TCP.TotalRetrans"`

	PacingRate    int64 `csv:"TCP.PacingRate"`
	MaxPacingRate int64 `csv:"TCP.MaxPacingRate"`

	BytesAcked    int64 `csv:"TCP.BytesAcked"`
	BytesReceived int64 `csv:"TCP.BytesReceived"`
	SegsOut       int32 `csv:"TCP.SegsOut"`
	SegsIn        int32 `csv:"TCP.SegsIn"`

	NotsentBytes uint32 `csv:"TCP.NotsentBytes"`
	MinRTT       uint32 `csv:"TCP.MinRTT"`
	DataSegsIn   uint32 `csv:"TCP.DataSegsIn"`
	DataSegsOut  uint32 `csv:"TCP.DataSegsOut"`

	DeliveryRate int64 `csv:"TCP.DeliveryRate"`

	BusyTime      int64 `csv:"TCP.BusyTime"`
	RWndLimited   int64 `csv:"TCP.RWndLimited"`
	SndBufLimited int64 `csv:"TCP.SndBufLimited"`

	Delivered   uint32 `csv:"TCP.Delivered"`
	DeliveredCE uint32 `csv:"TCP.DeliveredCE"`

	BytesSent    int64 `csv:"TCP.BytesSent"`
	BytesRetrans int64 `csv:"TCP.BytesRetrans"`

	DSackDups uint32 `csv:"TCP.DSackDups"`
	ReordSeen uint32 `csv:"TCP.ReordSeen"`

	// ElapsedTime is intentionally placed here (not last) to match the legacy
	// ndt7 BigQuery table; RcvOooPack and SndWnd follow it. Append new fields
	// AFTER SndWnd only.
	ElapsedTime int64

	RcvOooPack uint32 `csv:"TCP.RcvOooPack"`
	SndWnd     uint32 `csv:"TCP.SndWnd"`
}

// NewTCPInfo builds a TCPInfo from a tcp.LinuxTCPInfo sampled from the kernel.
func NewTCPInfo(ti tcp.LinuxTCPInfo, elapsed int64) *TCPInfo {
	return &TCPInfo{
		State:         ti.State,
		CAState:       ti.CAState,
		Retransmits:   ti.Retransmits,
		Probes:        ti.Probes,
		Backoff:       ti.Backoff,
		Options:       ti.Options,
		WScale:        ti.WScale,
		AppLimited:    ti.AppLimited,
		RTO:           ti.RTO,
		ATO:           ti.ATO,
		SndMSS:        ti.SndMSS,
		RcvMSS:        ti.RcvMSS,
		Unacked:       ti.Unacked,
		Sacked:        ti.Sacked,
		Lost:          ti.Lost,
		Retrans:       ti.Retrans,
		Fackets:       ti.Fackets,
		LastDataSent:  ti.LastDataSent,
		LastAckSent:   ti.LastAckSent,
		LastDataRecv:  ti.LastDataRecv,
		LastAckRecv:   ti.LastAckRecv,
		PMTU:          ti.PMTU,
		RcvSsThresh:   ti.RcvSsThresh,
		RTT:           ti.RTT,
		RTTVar:        ti.RTTVar,
		SndSsThresh:   ti.SndSsThresh,
		SndCwnd:       ti.SndCwnd,
		AdvMSS:        ti.AdvMSS,
		Reordering:    ti.Reordering,
		RcvRTT:        ti.RcvRTT,
		RcvSpace:      ti.RcvSpace,
		TotalRetrans:  ti.TotalRetrans,
		PacingRate:    ti.PacingRate,
		MaxPacingRate: ti.MaxPacingRate,
		BytesAcked:    ti.BytesAcked,
		BytesReceived: ti.BytesReceived,
		SegsOut:       ti.SegsOut,
		SegsIn:        ti.SegsIn,
		NotsentBytes:  ti.NotsentBytes,
		MinRTT:        ti.MinRTT,
		DataSegsIn:    ti.DataSegsIn,
		DataSegsOut:   ti.DataSegsOut,
		DeliveryRate:  ti.DeliveryRate,
		BusyTime:      ti.BusyTime,
		RWndLimited:   ti.RWndLimited,
		SndBufLimited: ti.SndBufLimited,
		Delivered:     ti.Delivered,
		DeliveredCE:   ti.DeliveredCE,
		BytesSent:     ti.BytesSent,
		BytesRetrans:  ti.BytesRetrans,
		DSackDups:     ti.DSackDups,
		ReordSeen:     ti.ReordSeen,
		ElapsedTime:   elapsed,
		RcvOooPack:    ti.RcvOooPack,
		SndWnd:        ti.SndWnd,
	}
}
