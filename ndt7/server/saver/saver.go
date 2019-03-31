// Package saver saves measurements coming from the reader and the writer
// of a specific subtest into a results file.
package saver

import (
	"sync"

	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/server/results"
)

// imsg is the saver's internal representation of a measurement.
type imsg struct {
	// o is the measurement origin (either "server" or "client")
	o string

	// m is the measurement itself.
	m model.Measurement
}

// zip takes in input two channels, one emitting measurements performed by the
// client, the other emitting measurements performed by the server, and emits
// internal messages where the measurement origin is explicit. The saver is then
// expected to drain the returned channel and properly save measurements.
func zip(serverch, clientch <-chan model.Measurement) <-chan imsg {
	// Implementation note: the follwing is the well known golang
	// pattern for joining channels together
	outch := make(chan imsg)
	var wg sync.WaitGroup
	wg.Add(2)
	// serverch; note that it MUST provide a liveness guarantee
	go func(out chan<- imsg) {
		for m := range(serverch) {
			out <- imsg{o: "server", m: m}
		}
		wg.Done()
	}(outch)
	// clientch; note that it MUST provide a liveness guarantee
	go func(out chan<- imsg) {
		for m := range(clientch) {
			out <- imsg{o: "client", m: m}
		}
		wg.Done()
	}(outch)
	// closer; will always terminate because of above liveness guarantees
	go func() {
		logging.Logger.Debug("saver: start waiting for clientch and serverch")
		defer logging.Logger.Debug("saver: stop waiting for clientch and serverch")
		wg.Wait()
		close(outch)
	}()
	return outch
}

// SaveAll saves measurements coming from the channel gathering client
// measurements (clientch) and from the channel gathering server measurements
// (serverch) into the results file (resultfp).
func SaveAll(serverch, clientch <-chan model.Measurement, resultfp *results.File) {
	zipch := zip(serverch, clientch)
	defer func() {
		logging.Logger.Debug("saver: start draining zipch")
		defer logging.Logger.Debug("saver: stop draining zipch")
		for range zipch {
			// make sure we drain the channel if we leave the loop below early
			// because we cannot save some results
		}
	}()
	logging.Logger.Debug("saver: start")
	defer logging.Logger.Debug("saver: stop")
	for imsg := range zipch {
		if err := resultfp.WriteMeasurement(imsg.m, imsg.o); err != nil {
			logging.Logger.WithError(err).Warn(
				"saver: resultfp.WriteMeasurement failed",
			)
			break
		}
	}
}
