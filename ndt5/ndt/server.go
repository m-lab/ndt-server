package ndt

import (
	"context"

	"github.com/m-lab/ndt-server/ndt5/protocol"
)

// ConnectionType records whether this test is performed over plain TCP,
// websockets, or secure websockets.
type ConnectionType string

// String returns the connection type named used in archival data.
func (c ConnectionType) String() string {
	return string(c)
}

// Label returns the connection type name used in monitoring metrics.
func (c ConnectionType) Label() string {
	switch c {
	case WSS:
		return "ndt5+wss"
	case WS:
		return "ndt5+ws"
	case Plain:
		return "ndt5+plain"
	default:
		return "ndt5+unknown"
	}
}

// The types of connections we support.
var (
	WS    = ConnectionType("WS")
	WSS   = ConnectionType("WSS")
	Plain = ConnectionType("PLAIN")
)

// Server describes the methods implemented by every server of every connection
// type.
type Server interface {
	SingleMeasurementServerFactory
	ConnectionType() ConnectionType
	DataDir() string
	LoginCeremony(protocol.Connection) (int, error)
}

// SingleMeasurementServerFactory is the method by which we abstract away what
// kind of server is being created at any given time. Using this abstraction
// allows us to use almost the same code for WS and WSS.
type SingleMeasurementServerFactory interface {
	SingleServingServer(direction string) (SingleMeasurementServer, error)
}

// SingleMeasurementServer is the interface implemented by every single-serving
// server. No matter whether they use WSS, WS, TCP with JSON, or TCP without
// JSON.
type SingleMeasurementServer interface {
	Port() int
	ServeOnce(context.Context) (protocol.MeasuredConnection, error)
	Close()
}
