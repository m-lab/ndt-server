// Package saver contains the code for saving results.
//
// Generally speaking, a ndt7 download or upload tests consists of two
// independent streams of data that need to be saved.
//
// During the download, there is a stream of Measurements performed by the
// server that contain TCPInfo and BBRInfo. For full duplex clients, we also
// have a stream of application-level measurements performed by the client
// that contain timestamps and number of received bytes.
//
// During the upload, there is a stream of Measurements performed by the
// server that contain TCPInfo, timestamps, and number of bytes that we
// have received so far. Clients implementing BBR or at least using Linux
// will additionally send us BBR and TCPInfo measurements.
//
// Thus, in both cases we need to have a functionality for zipping
// together the channel of measurements originating at the client and
// the channel originating at the server. This is the saver.
package saver

import (
	"sync"

	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/results"
)

// ismg is an internal message.
type imsg struct {
	// o is the origin (either "client" or "server").
	o string

	// m is the measurement.
	m model.Measurement
}

// zip zips the channel of the measurements performed by the server (i.e.
// serverch) and the one of the measurements from the client (i.e. clientch)
// and posts them onto the returned channel.
//
// This function assumes that serverch and clientch provide some liveness
// and deadlock free guarantees (i.e. that they will eventually terminate and
// will never block forever). Provided that this assumption is true, then
// this function will return when both channels are closed.
func zip(serverch, clientch <-chan model.Measurement) <-chan imsg {
	// Implementation note: the follwing is the well known golang
	// pattern for joining channels together
	outch := make(chan imsg)
	var wg sync.WaitGroup
	wg.Add(2)
	// serverch; note that it MUST provide a liveness guarantee
	go func(out chan<- imsg) {
		for m := range serverch {
			out <- imsg{o: "server", m: m}
		}
		wg.Done()
	}(outch)
	// clientch; note that it MUST provide a liveness guarantee
	go func(out chan<- imsg) {
		for m := range clientch {
			out <- imsg{o: "client", m: m}
		}
		wg.Done()
	}(outch)
	// closer; will always terminate because of above liveness guarantees
	go func() {
		logging.Logger.Debug("saver: start waiting for server and client")
		defer logging.Logger.Debug("saver: stop waiting for server and client")
		wg.Wait()
		close(outch)
	}()
	return outch
}

// SaveAll saves all the measurements coming from the channel where server
// performed measurements are posted (serverch) and from the channel where
// client performed measurements are posted (clientch). Measurements will
// be saved in the results file (resultfp).
//
// In any case, the input channels will be drained by this function. The input
// channels must have the following properties:
//
// 1. they MUST be closed when done
//
// 2. they MUST eventually terminate
//
// If these two properties are satisfied, SaveAll will eventually terminate.
func SaveAll(resultfp *results.File, serverch, clientch <-chan model.Measurement) {
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
		// We don't want to save connection_info on the server side because that's
		// just convenience information provided to the client that is already
		// duplicated into the "header" that we add as first line of a results file.
		if imsg.m.ConnectionInfo != nil {
			imsg.m.ConnectionInfo = nil
		}
		if err := resultfp.WriteMeasurement(imsg.m, imsg.o); err != nil {
			logging.Logger.WithError(err).Warn(
				"saver: resultfp.WriteMeasurement failed",
			)
			break
		}
	}
}
