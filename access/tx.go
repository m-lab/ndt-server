package access

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/procfs"
)

var (
	procPath         = "/proc"
	device           string
	txAccessRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_access_txcontroller_requests_total",
			Help: "Total number of requests handled by the access txcontroller.",
		},
		[]string{"request", "protocol"},
	)
	// ErrNoDevice is returned when device is empty or not found in procfs.
	ErrNoDevice = errors.New("no device found")
)

func init() {
	flag.StringVar(&device, "txcontroller.device", "", "Calculate bytes transmitted from this device.")
}

// TxController calculates the bytes transmitted every period from the named device.
type TxController struct {
	period  time.Duration
	device  string
	current uint64
	limit   uint64
	pfs     procfs.FS
}

// NewTxController creates a new instance initialized to run every second.
// Caller should run Watch in a goroutine to regularly update the current rate.
func NewTxController(rate uint64) (*TxController, error) {
	if device == "" {
		return nil, ErrNoDevice
	}
	pfs, err := procfs.NewFS(procPath)
	if err != nil {
		return nil, err
	}
	// Read the device once to verify that the device exists.
	_, err = readNetDevLine(pfs, device)
	if err != nil {
		return nil, err
	}
	tx := &TxController{
		device: device,
		limit:  rate,
		pfs:    pfs,
		period: 100 * time.Millisecond,
	}
	return tx, nil
}

// Accept wraps the call to listener's Accept. If the TxController is
// limited, then Accept immediately closes the connection and returns an error.
func (tx *TxController) Accept(l net.Listener) (net.Conn, error) {
	conn, err := l.Accept()
	if tx == nil {
		// Simple pass-through.
		return conn, err
	}
	if err == nil && tx.isLimited("raw") {
		defer conn.Close()
		return nil, fmt.Errorf("TxController rejected connection %s", conn.RemoteAddr())
	}
	return conn, err
}

// Current exports the current rate. Useful for diagnostics.
func (tx *TxController) Current() uint64 {
	return atomic.LoadUint64(&tx.current)
}

// isLimited checks the current tx rate and returns whether the connection should
// be accepted or rejected.
func (tx *TxController) isLimited(proto string) bool {
	cur := atomic.LoadUint64(&tx.current)
	if tx.limit > 0 && cur > tx.limit {
		txAccessRequests.WithLabelValues("rejected", proto).Inc()
		return true
	}
	txAccessRequests.WithLabelValues("accepted", proto).Inc()
	return false
}

// Limit enforces that the TxController rate limit is respected before running
// the next handler. If the rate is unspecified (zero), all requests are accepted.
func (tx *TxController) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tx.isLimited("http") {
			// 503 - https://tools.ietf.org/html/rfc7231#section-6.6.4
			w.WriteHeader(http.StatusServiceUnavailable)
			// Return without additional response.
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Watch updates the current rate every period. If the context is cancelled, the
// context error is returned. If the TxController rate is zero, Watch returns
// immediately. Callers should typically run Watch in a goroutine.
func (tx *TxController) Watch(ctx context.Context) error {
	if tx.limit == 0 {
		// No need to do anything.
		return nil
	}
	t := time.NewTicker(tx.period)
	defer t.Stop()

	// Read current vaule of TxBytes for device to initialize the following loop.
	v, err := readNetDevLine(tx.pfs, tx.device)
	if err != nil {
		return err
	}

	// Check the device every period until the context returns an error.
	ratePrev := 0.0
	prevTxBytes := v.TxBytes
	for ; ctx.Err() == nil; <-t.C {
		cur, err := readNetDevLine(tx.pfs, tx.device)
		if err != nil {
			log.Println("Error reading /proc/net/dev:", err)
			continue
		}

		// Calculate the new rate in bits-per-second.
		rateNow := float64(8*(cur.TxBytes-prevTxBytes)) / tx.period.Seconds()
		// A few seconds for decreases and rapid response for increases.
		ratePrev = math.Max(rateNow, 0.95*ratePrev+0.05*rateNow)
		atomic.StoreUint64(&tx.current, uint64(ratePrev))

		// Save the total bytes sent from this round for the next.
		prevTxBytes = cur.TxBytes
	}
	return ctx.Err()
}

func readNetDevLine(pfs procfs.FS, device string) (procfs.NetDevLine, error) {
	nd, err := pfs.NetDev()
	if err != nil {
		return procfs.NetDevLine{}, err
	}
	// Check at creation time whether device exists.
	v, ok := nd[device]
	if !ok {
		return procfs.NetDevLine{}, fmt.Errorf("Given device not found: %q", device)
	}
	return v, nil
}
