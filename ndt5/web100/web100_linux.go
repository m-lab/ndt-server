package web100

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/m-lab/ndt-server/tcpinfox"
	"github.com/m-lab/tcp-info/tcp"
)

func summarize(snaps []*tcp.LinuxTCPInfo) (*Metrics, error) {
	if len(snaps) == 0 {
		return nil, errors.New("zero-length list of data collected")
	}
	minrtt := uint32(0)
	for _, snap := range snaps {
		if snap.RTT < minrtt || minrtt == 0 {
			minrtt = snap.RTT
		}
	}
	info := &Metrics{
		TCPInfo: *snaps[len(snaps)-1], // Save the last snapshot into the metric struct.
		MinRTT:  minrtt / 1000,        // Convert microseconds to milliseconds for legacy compatibility.
	}
	log.Println("Summarized data:", info)
	return info, nil
}

// MeasureViaPolling collects all required data by polling. It is required for
// non-BBR connections because MinRTT is one of our critical metrics.
func MeasureViaPolling(ctx context.Context, fp *os.File, c chan *Metrics) {
	defer close(c)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	snaps := make([]*tcp.LinuxTCPInfo, 0, 100)
	// Poll until the context is canceled.
	for {
		// Get the tcp_cc metrics
		info, err := tcpinfox.GetTCPInfo(fp)
		if err == nil {
			snaps = append(snaps, info)
		} else {
			log.Println("Getsockopt error:", err)
		}
		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			info, err := summarize(snaps)
			if err == nil {
				c <- info
			}
			return
		}
	}
}

// TODO: Implement BBR support for ndt5 clients.
/*
func MeasureBBR(ctx context.Context, fp *os.File) (Metrics, error) {
	return Metrics{}, errors.New("MeasureBBR is unimplemented")
}
*/
