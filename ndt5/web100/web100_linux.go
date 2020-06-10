package web100

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/m-lab/ndt-server/netx"
	"github.com/m-lab/tcp-info/tcp"
)

func summarize(snaps []tcp.LinuxTCPInfo) (*Metrics, error) {
	if len(snaps) == 0 {
		return nil, errors.New("zero-length list of data collected")
	}
	sumrtt := uint32(0)
	countrtt := uint32(0)
	maxrtt := uint32(0)
	minrtt := uint32(0)
	for _, snap := range snaps {
		countrtt++
		sumrtt += snap.RTT
		if snap.RTT < minrtt || minrtt == 0 {
			minrtt = snap.RTT
		}
		if snap.RTT > maxrtt {
			maxrtt = snap.RTT
		}
	}
	lastSnap := snaps[len(snaps)-1]
	info := &Metrics{
		TCPInfo: snaps[len(snaps)-1], // Save the last snapshot of TCPInfo data into the metric struct.

		MinRTT: minrtt / 1000, // tcpinfo is microsecond data, web100 needs milliseconds
		MaxRTT: maxrtt / 1000, // tcpinfo is microsecond data, web100 needs milliseconds
		SumRTT: sumrtt / 1000, // tcpinfo is microsecond data, web100 needs milliseconds

		CountRTT: countrtt, // This counts how many samples went into SumRTT

		CurMSS: lastSnap.SndMSS,

		// If this cast bites us, it's because of a 10 second test pushing more than
		//  2**31 packets * 1500 bytes/packet * 8 bits/byte / 10 seconds = 2,576,980,377,600 bits/second = 2.5Tbps
		// If we are using web100 variables to measure terabit connections then
		// something has gone horribly wrong. Please switch to NDT7+tcpinfo or
		// whatever their successor is.
		PktsOut: uint32(lastSnap.SegsOut),
	}
	return info, nil
}

func measureUntilContextCancellation(ctx context.Context, ci netx.ConnInfo) (*Metrics, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	// We need to make sure fp is closed when the polling loop ends to ensure legacy
	// clients work. See https://github.com/m-lab/ndt-server/issues/160.
	defer ticker.Stop()

	snaps := make([]tcp.LinuxTCPInfo, 0, 200) // Enough space for 20 seconds of data.

	// Poll until the context is canceled, but never more than once per ticker-firing.
	//
	// This slightly-funny way of writing the loop ensures that one last
	// measurement occurs after the context is canceled (unless the most recent
	// measurement and the context cancellation happened simultaneously, in which
	// case the most recent measurement should count as the last measurement).
	for ; ctx.Err() == nil; <-ticker.C {
		// Get the tcp_cc metrics
		_, snapshot, err := ci.ReadInfo()
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
func MeasureViaPolling(ctx context.Context, ci netx.ConnInfo) <-chan *Metrics {
	// Give a capacity of 1 because we will only ever send one message and the
	// buffer allows the component goroutine to exit when done, no matter what the
	// client does.
	c := make(chan *Metrics, 1)
	go func() {
		summary, err := measureUntilContextCancellation(ctx, ci)
		if err == nil {
			c <- summary
		}
		close(c)
	}()
	return c
}
