package ndt

import (
	"context"
	"time"

	"github.com/m-lab/ndt-server/legacy/protocol"
)

// ConnectionType records whether this test is performed over plain TCP,
// websockets, or secure websockets.
type ConnectionType string

// The types of connections we support.
var (
	WS    = ConnectionType("WS")
	WSS   = ConnectionType("WSS")
	Plain = ConnectionType("PLAIN")
)

// Server describes the methods implemented by every server of every connection
// type.
type Server interface {
	TestServerFactory
	ConnectionType() ConnectionType
	DataDir() string

	// TestLength allows us to create fake servers which run tests very quickly.
	TestLength() time.Duration
	// TestMaxTime allows us to create fake servers which run tests very quickly.
	TestMaxTime() time.Duration
}

// TestServerFactory is the method by which we abstract away what kind of server is being
// created at any given time. Using this abstraction allows us to use almost the
// same code for WS and WSS.
type TestServerFactory interface {
	SingleServingServer(direction string) (TestServer, error)
}

// TestServer is the interface implemented by every single-serving server. No
// matter whether they use WSS, WS, TCP with JSON, or TCP without JSON.
type TestServer interface {
	Port() int
	ServeOnce(context.Context) (protocol.MeasuredConnection, error)
	Close()
}
