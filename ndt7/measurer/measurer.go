// Package measurer collects metrics from a socket connection
// and returns them for consumption.
package measurer

import (
	"context"
	"time"

	"github.com/gorilla/websocket"

	"github.com/m-lab/go/memoryless"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/netx"
)

// Measurer performs measurements
type Measurer struct {
	conn   *websocket.Conn
	uuid   string
	ticker *memoryless.Ticker
}

// New creates a new measurer instance
func New(conn *websocket.Conn, UUID string) *Measurer {
	return &Measurer{
		conn: conn,
		uuid: UUID,
	}
}

func (m *Measurer) getSocketAndPossiblyEnableBBR() (netx.ConnInfo, error) {
	ci := netx.ToConnInfo(m.conn.UnderlyingConn())
	err := ci.EnableBBR()
	if err != nil {
		logging.Logger.WithError(err).Warn("Cannot enable BBR")
		// FALLTHROUGH
	}
	return ci, nil
}

func measure(measurement *model.Measurement, ci netx.ConnInfo, elapsed time.Duration) {
	// Implementation note: we always want to sample BBR before TCPInfo so we
	// will know from TCPInfo if the connection has been closed.
	t := int64(elapsed / time.Microsecond)
	bbrinfo, tcpInfo, err := ci.ReadInfo()
	if err == nil {
		measurement.BBRInfo = &model.BBRInfo{
			BBRInfo:     bbrinfo,
			ElapsedTime: t,
		}
		measurement.TCPInfo = &model.TCPInfo{
			LinuxTCPInfo: tcpInfo,
			ElapsedTime:  t,
		}
	}
}

func (m *Measurer) loop(ctx context.Context, timeout time.Duration, dst chan<- model.Measurement) {
	logging.Logger.Debug("measurer: start")
	defer logging.Logger.Debug("measurer: stop")
	defer close(dst)
	measurerctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ci, err := m.getSocketAndPossiblyEnableBBR()
	if err != nil {
		logging.Logger.WithError(err).Warn("getSocketAndPossiblyEnableBBR failed")
		return
	}
	start := time.Now()
	connectionInfo := &model.ConnectionInfo{
		Client: m.conn.RemoteAddr().String(),
		Server: m.conn.LocalAddr().String(),
		UUID:   m.uuid,
	}
	// Implementation note: the ticker will close its output channel
	// after the controlling context is expired.
	ticker, err := memoryless.NewTicker(measurerctx, memoryless.Config{
		Min:      spec.MinPoissonSamplingInterval,
		Expected: spec.AveragePoissonSamplingInterval,
		Max:      spec.MaxPoissonSamplingInterval,
	})
	if err != nil {
		logging.Logger.WithError(err).Warn("memoryless.NewTicker failed")
		return
	}
	m.ticker = ticker
	for now := range ticker.C {
		var measurement model.Measurement
		measure(&measurement, ci, now.Sub(start))
		measurement.ConnectionInfo = connectionInfo
		dst <- measurement // Liveness: this is blocking
	}
}

// Start runs the measurement loop in a background goroutine and emits
// the measurements on the returned channel.
//
// Liveness guarantee: the measurer will always terminate after
// the given timeout, provided that the consumer continues reading from the
// returned channel. Measurer may be stopped early by canceling ctx, or by
// calling Stop.
func (m *Measurer) Start(ctx context.Context, timeout time.Duration) <-chan model.Measurement {
	dst := make(chan model.Measurement)
	go m.loop(ctx, timeout, dst)
	return dst
}

// Stop ends the measurements and drains the measurement channel. Stop
// guarantees that the measurement goroutine completes by draining the
// measurement channel. Users that call Start should also call Stop.
func (m *Measurer) Stop(src <-chan model.Measurement) {
	if m.ticker != nil {
		m.ticker.Stop()
	}
	for range src {
		// make sure we drain the channel, so the measurement loop can exit.
	}
}
