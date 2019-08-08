package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics for general use, in both NDT5 and in NDT7.
var (
	ActiveTests = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ndt_active_tests",
			Help: "A gauge of requests currently being served by the NDT server.",
		},
		[]string{"protocol"})
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
		[]string{"protocol", "direction"},
	)
)
