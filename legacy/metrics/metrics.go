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
				100, 150},
		},
		[]string{"protocol"},
	)
	MeasurementServerStart = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_legacy_measurementserver_start_total",
			Help: "The number of times a single-serving server was started.",
		},
		[]string{"protocol"},
	)
	MeasurementServerStop = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_legacy_measurementserver_stop_total",
			Help: "The number of times a single-serving server was stopped.",
		},
		[]string{"protocol"},
	)
	SniffedReverseProxyCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "ndt_legacy_sniffed_ws_total",
			Help: "The number of times we sniffed-then-proxied a websocket connection on the legacy channel.",
		},
	)
)
