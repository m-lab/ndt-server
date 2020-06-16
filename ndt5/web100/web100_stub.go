// +build !linux

package web100

import (
	"context"

	"github.com/m-lab/ndt-server/netx"
)

// MeasureViaPolling collects all required data by polling. It is required for
// non-BBR connections because MinRTT is one of our critical metrics.
func MeasureViaPolling(ctx context.Context, ci netx.ConnInfo) <-chan *Metrics {
	// Just a stub.
	return nil
}
