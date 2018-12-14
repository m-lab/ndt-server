// Package spec contains constants defined in the ndt7 specification.
package spec

import "time"

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
