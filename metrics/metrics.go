package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// CurrentTests keeps track of how many tests are currently executing (and what type they are)
	CurrentTests = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ndt_control_current",
			Help: "A gauge of requests currently being served by the NDT control handler.",
		},
		[]string{"type"})
	// TestDuration tracks, for each test type and success code, how long each test took.
	TestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "ndt_control_duration_seconds",
			Help: "A histogram of request latencies to the control channel.",
			// Durations will likely be tri-modal: early failures (fast),
			// completed single test (slower), completed dual tests (slowest) or timeouts.
			Buckets: []float64{.1, 1, 10, 10.5, 11, 11.5, 12, 20, 21, 22, 30, 60},
		},
		[]string{"type", "code"},
	)
)
