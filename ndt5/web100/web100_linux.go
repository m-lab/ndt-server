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
		TCPInfo: *snaps[len(snaps)-1], // Save the last snapshot of TCPInfo data into the metric struct.
		MinRTT:  minrtt / 1000,        // Convert microseconds to milliseconds for legacy compatibility.
	}
	return info, nil
}

func measureUntilContextCancellation(ctx context.Context, fp *os.File) (*Metrics, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	snaps := make([]*tcp.LinuxTCPInfo, 0, 200) // Enough space for 20 seconds of data.

	// Poll until the context is canceled, but never more than once per ticker-firing.
	//
	// This slightly-funny way of writing the loop ensures that one last
	// measurement occurs after the context is canceled (unless the most recent
	// measurement and the context cancellation happened simultaneously, in which
	// case the most recent measurement should count as the last measurement).
	for ; ctx.Err() == nil; <-ticker.C {
		// Get the tcp_cc metrics
		snapshot, err := tcpinfox.GetTCPInfo(fp)
		if err == nil {
			snaps = append(snaps, snapshot)
		} else {
			log.Println("Getsockopt error:", err)
		}
	}
	return summarize(snaps)
}

// MeasureViaPolling collects all required data by polling and returns a channel
// for the results. This function may or may not send socket information along
// the channel, depending on whether or not an error occurred. The value is sent
// along the channel sometime after the context is canceled.
func MeasureViaPolling(ctx context.Context, fp *os.File) <-chan *Metrics {
	// Give a capacity of 1 because we will only ever send one message and the
	// buffer allows the component goroutine to exit when done, no matter what the
	// client does.
	c := make(chan *Metrics, 1)
	go func() {
		summary, err := measureUntilContextCancellation(ctx, fp)
		if err == nil {
			c <- summary
		}
		close(c)
	}()
	return c
}
