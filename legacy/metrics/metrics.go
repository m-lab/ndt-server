package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// TestRate exports a histogram of request rates using prometheus
	TestRate = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "ndt_test_rate_mbps",
			Help: "A histogram of request rates.",
			Buckets: []float64{
				.1, .15, .25, .4, .6,
				1, 1.5, 2.5, 4, 6,
				10, 15, 25, 40, 60,
				100, 150, 250, 400, 600,
				1000},
		},
		[]string{"direction"},
	)
	// TestCount exports via prometheus the number of tests run by this server.
	TestCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_test_total",
			Help: "Number of NDT tests run by this server.",
		},
		[]string{"direction", "code"},
	)
	// ErrorCount exports the number of test failures seen by the server.
	ErrorCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_test_errors_total",
			Help: "Number of test errors of each type for each test.",
		},
		[]string{"test", "error"},
	)
)
