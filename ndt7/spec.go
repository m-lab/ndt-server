// Package ndt7 contains a non-backwards compatible redesign of the NDT
// network performance measurement protocol. In particular we redesigned the
// NDT protocol (NDP) to work natively and only over WebSocket and TLS, so
// to remove the complexity induced by trying to be backward compatible with
// NDT's original implementation (see https://github.com/ndt-project/ndt).
//
// The protocol described in this go documentation aims to become version
// 7.0.0 of the NDT protocol and aims to eventually live outside of this
// specification. This is why, optimistically, we will call the protocol NDT7
// or NDP7 in the remainder of the documentation.
//
// Protocol description
//
// The client connects to the server using TLS and requests to upgrade the
// connection to WebSockets. The same connection will be used to exchange
// control and measurement messages. The upgrade request URL will indicate
// the type of subtest that the client wants to perform. Two subtests and
// hence two URLs are defined:
//
// "/ndt/v7/download" selects the download subtest.
//
//   /ndt/v7/download
//
// "/ndt/v7/upload" selects the upload subtest.
//
//   /ndt/v7/upload
//
// The upgrade message MUST also contain the WebSocket subprotocol that
// identifies NDT7, which is "net.measurementlab.ndt.v7". The URL in the
// upgrade request MAY contain optional parameters for configuring the
// network test. An upgrade request could look like this:
//
//   GET /ndt/v7/download?duration=7.0 HTTP/1.1\r\n
//   Host: localhost\r\n
//   Connection: Upgrade\r\n
//   Sec-WebSocket-Key: DOdm+5/Cm3WwvhfcAlhJoQ==\r\n
//   Sec-WebSocket-Version: 13\r\n
//   Sec-WebSocket-Protocol: net.measurementlab.ndt.v7\r\n
//   Upgrade: websocket\r\n
//   \r\n
//
// Upon receiving the upgrade request, the server should check the
// parameters and either (1) respond with a 400 failure status code
// if the parameters are not okay or (2) upgrade the connection to
// WebSocket if parameters are acceptable. The upgrade response MUST
// contain the selected subprotocol in compliance with RFC6455. A
// possible upgrade response could look like this:
//
//   HTTP/1.1 101 Switching Protocols\r\n
//   Sec-WebSocket-Protocol: net.measurementlab.ndt.v7\r\n
//   Sec-WebSocket-Accept: Nhz+x95YebD6Uvd4nqPC2fomoUQ=\r\n
//   Upgrade: websocket\r\n
//   Connection: Upgrade\r\n
//   \r\n
//
// Once the WebSocket channel is established, the client and the server
// exchange NDT7 messages using the WebSocket framing. An implementation MAY
// choose to limit the maximum WebSocket message size, but such limit MUST
// NOT be smaller than 1 << 17 bytes.
//
// Binary WebSocket messages will carry a body composed of random bytes and
// will be used to measure the network performance. In the download subtest
// these messages are sent by the server to the client. In the upload
// subtest the client will send binary messages to the server. If a binary
// message is received when it is not expected (i.e. the server receives
// a binary message during the download) the connection SHOULD be closed.
//
// Textual WebSocket messages will contain serialized JSON stuctures containing
// measurement results. This kind of messages MAY be sent by both the client
// and the server throughout the subtest, regardless of the test type, because
// both parties run network measurements they MAY want to share. Note: the
// bytes exhanged as part of the textual messages could themselves be useful
// to measure the network performance.
//
// When the configured duration time has expired, the parties SHOULD close
// the WebSocket channel by sending a Close WebSocket frame. The client
// SHOULD not close the TCP connection immediately, so that the server can
// close it first. This allows to reuse ports more efficiently on the
// server because we avoid TIME_WAIT.
package ndt7

import "time"

// DownloadURLPath selects the download subtest.
const DownloadURLPath = "/ndt/v7/download"

// UploadURLPath selects the upload subtest.
const UploadURLPath = "/ndt/v7/upload"

// SecWebSocketProtocol is the WebSocket subprotocol used by NDT7.
const SecWebSocketProtocol = "net.measurementlab.ndt.v7"

// MinMaxMessageSize is the minimum value of the maximum message size
// that an implementation MAY want to configure. Messages smaller than this
// threshold MUST always be accepted by an implementation.
const MinMaxMessageSize = 1 << 17

// Options are the options that can be configuered as part of the WebSocket
// upgrade using the query string. If options are not provided, sensible
// defaults SHOULD be selected by the server.
type Options struct {
	// Adaptive indicates whether we are allowed to stop the download early
	// if it's safe to do so according to BBR instrumentation, because we have
	// correctly estimated the available bandwidth.
	Adaptive bool
	// Duration is the expected duration (in seconds) of the subtest.
	Duration int
}

// The BBRInfo struct contains information measured using BBR.
type BBRInfo struct {
	// Bandwidth is the bandwidth measured by BBR in bytes/s.
	Bandwidth float64 `json:"bandwidth"`

	// RTT is the RTT measured by BBR in microseconds.
	RTT float64 `json:"rtt"`
}

// The Measurement struct contains measurement results. This structure is
// meant to be serialised as JSON as sent on a textual message.
type Measurement struct {
	// Number of nanoseconds elapsed since the beginning of the subtest.
	Elapsed int64 `json:"elapsed"`

	// Number of bytes transferred since the beginning of the subtest.
	NumBytes int64 `json:"num_bytes"`

	// Data measured using TCP BBR instrumentation.
	BBRInfo *BBRInfo `json:"bbr_info"`
}

// MinMeasurementInterval is the minimum value of the interval betwen
// two consecutive measurements performed by either party. An implementation
// MAY choose to close the connection if it is receiving too frequent
// Measurement messages from the other endpoint.
const MinMeasurementInterval = time.Duration(250) * time.Millisecond
