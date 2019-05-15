package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics for exporting to prometheus to aid in server monitoring.
//
// TODO: Decide what monitoring we want and transition to that.
var (
	ControlChannelDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "ndt_legacy_control_channel_duration",
			Help: "How long do tests last.",
			Buckets: []float64{
				.1, .15, .25, .4, .6,
				1, 1.5, 2.5, 4, 6,
				10, 15, 25, 40, 60,
				100, 150, 250, 400, 600,
				1000},
		},
		[]string{"protocol"},
	)
	ActiveTests = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ndt_legacy_active_tests",
			Help: "The number of tests currently running",
		},
		[]string{"protocol"},
	)
	Failures = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ndt_legacy_failures_total",
			Help: "The number of test failures",
		},
		[]string{"protocol", "error"},
	)
	TestServerStart = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_testserver_start_total",
			Help: "Number times a single-serving server was started.",
		},
		[]string{"protocol"},
	)
	TestServerStop = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_testserver_stop_total",
			Help: "Number times a single-serving server was stopped.",
		},
		[]string{"protocol"},
	)
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
	TestCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_test_total",
			Help: "Number of NDT tests run by this server.",
		},
		[]string{"direction", "code"},
	)
	ErrorCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_test_errors_total",
			Help: "Number of test errors of each type for each test.",
		},
		[]string{"test", "error"},
	)
)
