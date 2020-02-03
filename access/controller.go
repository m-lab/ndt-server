package access

import (
	"net/http"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	currentRequests = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ndt_access_maxcontroller_current",
			Help: "Current number of requests handled by the access maxcontroller.",
		},
	)
)

// MaxController controls the total number of clients that may run simultaneously.
// May be used on handlers for multiple servers.
type MaxController struct {
	Max     int64
	Current int64
}

// Limit enforces the Concurrent Max limit while running the next handler.
func (c *MaxController) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt64(&c.Current, 1)
		currentRequests.Set(float64(cur))
		defer func() {
			cur := atomic.AddInt64(&c.Current, -1)
			currentRequests.Set(float64(cur))
		}()
		if c.Max > 0 && cur > c.Max {
			// 503 - https://tools.ietf.org/html/rfc7231#section-6.6.4
			w.WriteHeader(http.StatusServiceUnavailable)
			// Return without additional response.
			return
		}
		next.ServeHTTP(w, r)
	})
}
