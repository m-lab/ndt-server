package access

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/procfs"
)

var (
	procPath = "/proc"
	device   string
)

func init() {
	flag.StringVar(&device, "txcontroller.device", "eth0", "Calculate bytes transmitted from this device.")
}

// TxController calculates the bytes transmitted every period from the named device.
type TxController struct {
	period  time.Duration
	device  string
	current uint64
	rate    uint64
	initial uint64
	pfs     procfs.FS
	handler http.Handler
}

// NewTxController creates a new instance initialized to run every period.
func NewTxController(rate uint64, period time.Duration) (*TxController, error) {
	pfs, err := procfs.NewFS(procPath)
	if err != nil {
		return nil, err
	}
	// Read the device once to initialize the TxBytes count.
	nd, err := pfs.NetDev()
	if err != nil {
		return nil, err
	}
	v, ok := nd[device]
	if !ok {
		return nil, fmt.Errorf("Given device not found: %q", device)
	}
	tx := &TxController{
		device:  device,
		initial: v.TxBytes,
		rate:    rate,
		pfs:     pfs,
		period:  period,
	}
	return tx, err
}

// Limit enforces that the TxController rate limit is respected before running
// the next handler.
func (tx *TxController) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.LoadUint64(&tx.current)
		if tx.rate > 0 && cur > tx.rate {
			// 503 - https://tools.ietf.org/html/rfc7231#section-6.6.4
			w.WriteHeader(http.StatusServiceUnavailable)
			// Return without additional response.
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Watch updates the current rate every period. If the context is cancelled, the
// context error is returned.
func (tx *TxController) Watch(ctx context.Context) error {
	if tx.rate == 0 {
		// No need to do anything.
		return nil
	}
	t := time.NewTicker(tx.period)
	for prev := tx.initial; ctx.Err() == nil; <-t.C {
		netdev, err := tx.pfs.NetDev()
		if err != nil {
			log.Println("Error reading /proc/net/dev:", err)
			continue
		}
		v, ok := netdev[tx.device]
		if ok {
			cur := (v.TxBytes - prev) * 8
			atomic.StoreUint64(&tx.current, cur)
			prev = v.TxBytes
		}
	}
	return ctx.Err()
}
