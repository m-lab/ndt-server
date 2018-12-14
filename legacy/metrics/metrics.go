package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// TestRate exports a histogram of request rates using prometheus
	TestRate = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "ndt_test_rate_mbps",
			Help: "A histogram of request rates.",
			Buckets: []float64{
				1, 1.5, 2.5, 4, 6,
				10, 15, 25, 40, 60,
				100, 150, 250, 400, 600,
				1000},
		},
		[]string{"direction"},
	)
	// TestCount exports via prometheus the number of tests run by this server.
	TestCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_test_total",
			Help: "Number of NDT tests run by this server.",
		},
		[]string{"direction", "code"},
	)
)

func init() {
	prometheus.MustRegister(TestCount)
	prometheus.MustRegister(TestRate)
}
