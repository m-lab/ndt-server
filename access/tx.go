package access

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/prometheus/procfs"
)

var (
	procPath       = "/proc"
	device         string
	accessRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ndt_access_txcontroller_requests_total",
			Help: "Total number of requests handled by the access txcontroller.",
		},
		[]string{"request"},
	)
)

func init() {
	flag.StringVar(&device, "txcontroller.device", "eth0", "Calculate bytes transmitted from this device.")
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
		period: time.Second,
	}
	return tx, err
}

// Limit enforces that the TxController rate limit is respected before running
// the next handler. If the rate is unspecified (zero), all requests are accepted.
func (tx *TxController) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.LoadUint64(&tx.current)
		if tx.limit > 0 && cur > tx.limit {
			accessRequests.WithLabelValues("rejected").Inc()
			// 503 - https://tools.ietf.org/html/rfc7231#section-6.6.4
			w.WriteHeader(http.StatusServiceUnavailable)
			// Return without additional response.
			return
		}
		accessRequests.WithLabelValues("accepted").Inc() // accepted != success.
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
	for prev := v.TxBytes; ctx.Err() == nil; <-t.C {
		v, err := readNetDevLine(tx.pfs, tx.device)
		if err != nil {
			log.Println("Error reading /proc/net/dev:", err)
			continue
		}
		cur := (v.TxBytes - prev) * 8
		atomic.StoreUint64(&tx.current, cur)
		prev = v.TxBytes
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
