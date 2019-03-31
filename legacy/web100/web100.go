// Package web100 provides web100 variables (or a simulation thereof) to
// interested systems. When run on not-BBR it is polling-based, when run on BBR
// it only needs to measure once.
package web100

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// Metrics holds web100 data. According to the legacy NDT protocol, each of
// these metrics is required. That does not mean each is required to be
// non-zero, but it does mean that the field should be present in any response.
type Metrics struct {
	// Milliseconds
	MaxRTT, MinRTT, SumRTT, CurRTO, SndLimTimeCwnd, SndLimTimeRwin, SndLimTimeSender uint32

	// Counters
	DataBytesOut                                                           uint64
	DupAcksIn, PktsOut, PktsRetrans, Timeouts, CountRTT, CongestionSignals uint32
	AckPktsIn                                                              uint32 // Called SegsIn in tcp-kis.txt

	// Octets
	MaxCwnd, MaxRwinRcvd, CurMSS, Sndbuf uint32

	// Scaling factors
	RcvWinScale, SndWinScale int

	// Useful metrics that are not part of the required set.
	BytesPerSecond float64
}

func summarize(snaps []*unix.TCPInfo) (*Metrics, error) {
	if len(snaps) == 0 {
		return nil, errors.New("zero-length list of data collected")
	}
	minrtt := uint32(0)
	for _, snap := range snaps {
		if snap.Rtt < minrtt || minrtt == 0 {
			minrtt = snap.Rtt
		}
	}
	info := &Metrics{MinRTT: minrtt / 1000} // Convert microseconds to milliseconds.
	log.Println("Summarized data:", info)
	return info, nil
}

// MeasureViaPolling collects all required data by polling. It is required for
// non-BBR connections because MinRTT is one of our critical metrics.
func MeasureViaPolling(ctx context.Context, fp *os.File, c chan *Metrics) {
	defer close(c)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	snaps := make([]*unix.TCPInfo, 0, 100)
	// Poll until the context is canceled.
	for {
		// Get the tcp_cc metrics
		info, err := unix.GetsockoptTCPInfo(int(fp.Fd()), unix.IPPROTO_TCP, unix.TCP_INFO)
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

// TODO: Implement BBR support for legacy clients.
/*
func MeasureBBR(ctx context.Context, fp *os.File) (Metrics, error) {
	return Metrics{}, errors.New("MeasureBBR is unimplemented")
}
*/
