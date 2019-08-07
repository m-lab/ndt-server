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
			Name: "ndt5_control_channel_duration",
			Help: "How long do tests last.",
			Buckets: []float64{
				.1, .15, .25, .4, .6,
				1, 1.5, 2.5, 4, 6,
				10, 15, 25, 40, 60,
				100, 150},
		},
		[]string{"protocol"},
	)
	ControlPanicCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt5_control_panic_total",
			Help: "Number of recovered panics in the control channel.",
		},
		[]string{"type", "error"},
	)
	MeasurementServerStart = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt5_measurementserver_start_total",
			Help: "The number of times a single-serving server was started.",
		},
		[]string{"protocol"},
	)
	MeasurementServerStop = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt5_measurementserver_stop_total",
			Help: "The number of times a single-serving server was stopped.",
		},
		[]string{"protocol"},
	)
	SniffedReverseProxyCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "ndt5_sniffed_ws_total",
			Help: "The number of times we sniffed-then-proxied a websocket connection on the plain ndt5 channel.",
		},
	)
	ClientRequestedTestSuites = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt5_client_requested_suites_total",
			Help: "The number of client request test suites (the combination of all test types as an integer 0-255).",
		},
		[]string{"suite"},
	)
	ClientRequestedTests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt5_client_requested_tests_total",
			Help: "The number of client requests for each ndt5 test type.",
		},
		[]string{"type"},
	)
	SubmittedMetaValues = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "ndt5_submitted_meta_values",
			Help: "The number of meta values submitted by clients.",
			Buckets: []float64{
				0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
				11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		},
	)
)
