package metrics

import (
	"strings"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics for exporting to prometheus to aid in server monitoring.
var (
	ClientConnections = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt7_client_connections_total",
			Help: "Count of clients that connect and setup an ndt7 measurement.",
		},
		[]string{"direction", "status"},
	)
	ClientTestResults = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt7_client_test_results_total",
			Help: "Number of client-connections for NDT tests run by this server.",
		},
		[]string{"protocol", "direction", "result"},
	)
	ClientSenderErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt7_client_sender_errors_total",
			Help: "Number of sender errors on all return paths.",
		},
		[]string{"protocol", "direction", "error"},
	)
	ClientReceiverErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt7_client_receiver_errors_total",
			Help: "Number of receiver errors on all return paths.",
		},
		[]string{"protocol", "direction", "error"},
	)
)

// ConnLabel infers an appropriate label for the websocket protocol.
func ConnLabel(conn *websocket.Conn) string {
	// NOTE: this isn't perfect, but it is simple and a) works for production deployments,
	// and 2) will work for custom deployments with ports having the same suffix, e.g. 4433, 8080.
	if strings.HasSuffix(conn.LocalAddr().String(), "443") {
		return "ndt7+wss"
	}
	if strings.HasSuffix(conn.LocalAddr().String(), "80") {
		return "ndt7+ws"
	}
	return "ndt7+unknown"
}
