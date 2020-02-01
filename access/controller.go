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
	initial uint64
	pfs     procfs.FS
	handler http.Handler
}

// NewTxController creates a new instance initialized to run every period.
func NewTxController(period time.Duration) (*TxController, error) {
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
		pfs:     pfs,
		period:  period,
	}
	return tx, err
}

// Wrap..
func (tx *TxController) Wrap(h http.Handler) {
	tx.handler = h
}

func (tx *TxController) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if
	tx.handler.ServeHTTP(rw, req)
}

func (tx *TxController) Watch(ctx context.Context) error {
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
