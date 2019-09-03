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
	log.Println("Summarized data:", info)
	return info, nil
}

// MeasureViaPolling collects all required data by polling. This function, when
// it exits, will always close the passed-in channel. It may or may not send
// socket information along the channel before it closes, depending on whether
// or not an error occurred. If you want to avoid blocking and potential
// goroutine leaks (and you almost certainly do), then the passed-in channel
// should have a capacity of at least 1.
func MeasureViaPolling(ctx context.Context, fp *os.File, c chan *Metrics) {
	log.Println("Measuring via polling")
	defer close(c)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	snaps := make([]*tcp.LinuxTCPInfo, 0, 200) // Enough space for 20 seconds of data.
	// Poll until the context is canceled.
	for ctx.Err() == nil {
		// Get the tcp_cc metrics
		info, err := tcpinfox.GetTCPInfo(fp)
		if err == nil {
			snaps = append(snaps, info)
		} else {
			log.Println("Getsockopt error:", err)
		}
		// Wait for the next time interval
		<-ticker.C
	}
	info, err := summarize(snaps)
	if err == nil {
		c <- info
	}
}
