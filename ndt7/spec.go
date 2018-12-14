// Package ndt7 contains a non-backwards compatible redesign of the NDT
// network performance measurement protocol. The complete specification of
// the protocol is available at
// https://github.com/m-lab/ndt-cloud/blob/master/spec/ndt7.md.
package ndt7

import (
	"github.com/m-lab/ndt-cloud/ndt7/model"

	"time"
)

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

// MinMeasurementInterval is the minimum interval between measurements.
const MinMeasurementInterval = 250 * time.Millisecond

// The Measurement struct contains measurement results. This structure is
// meant to be serialised as JSON as sent on a textual message.
type Measurement struct {
	// Elapsed is the number of seconds elapsed since the beginning.
	Elapsed float64 `json:"elapsed"`

	// AppInfo contains application level measurements.
	AppInfo *model.AppInfo `json:"app_info,omitempty"`

	// BBRInfo is the data measured using TCP BBR instrumentation.
	BBRInfo *model.BBRInfo `json:"bbr_info,omitempty"`

	// TCPInfo contains metrics measured using TCP_INFO instrumentation.
	TCPInfo *model.TCPInfo `json:"tcp_info,omitempty"`
}
