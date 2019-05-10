package ndt

import (
	"time"

	"github.com/m-lab/ndt-server/legacy/singleserving"
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
	singleserving.Factory
	ConnectionType() ConnectionType
	DataDir() string

	// TestLength allows us to create fake servers which run tests very quickly.
	TestLength() time.Duration
	// TestMaxTime allows us to create fake servers which run tests very quickly.
	TestMaxTime() time.Duration
}
