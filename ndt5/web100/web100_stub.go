// +build !linux

package web100

import (
	"context"
	"os"
)

// MeasureViaPolling collects all required data by polling. It is required for
// non-BBR connections because MinRTT is one of our critical metrics.
func MeasureViaPolling(ctx context.Context, fp *os.File, c chan *Metrics) {
	// Just a stub.
}
