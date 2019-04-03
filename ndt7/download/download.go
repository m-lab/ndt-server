// Package download implements the ndt7/server downloader.
package download

import (
	"context"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/download/measurer"
	"github.com/m-lab/ndt-server/ndt7/download/receiver"
	"github.com/m-lab/ndt-server/ndt7/download/sender"
	"github.com/m-lab/ndt-server/ndt7/results"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

// Handler handles a download subtest from the server side.
type Handler struct {
	Upgrader websocket.Upgrader
	DataDir  string
}

func warnAndClose(writer http.ResponseWriter, message string) {
	logging.Logger.Warn(message)
	writer.Header().Set("Connection", "Close")
	writer.WriteHeader(http.StatusBadRequest)
}

type imsg struct {
	o string
	m model.Measurement
}

func zip(senderch, receiverch <-chan model.Measurement) <-chan imsg {
	// Implementation note: the follwing is the well known golang
	// pattern for joining channels together
	outch := make(chan imsg)
	var wg sync.WaitGroup
	wg.Add(2)
	// sender; note that it provides a liveness guarantee
	go func(out chan<- imsg) {
		for m := range(senderch) {
			out <- imsg{o: "server", m: m}
		}
		wg.Done()
	}(outch)
	// receiver; note that it provides a liveness guarantee
	go func(out chan<- imsg) {
		for m := range(receiverch) {
			out <- imsg{o: "client", m: m}
		}
		wg.Done()
	}(outch)
	// closer; will always terminate because of above liveness guarantees
	go func() {
		logging.Logger.Debug("download: start waiting for sender and receiver")
		defer logging.Logger.Debug("download: stop waiting for sender and receiver")
		wg.Wait()
		close(outch)
	}()
	return outch
}

// Handle handles the download subtest.
func (dl Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	logging.Logger.Debug("download: upgrading to WebSockets")
	if request.Header.Get("Sec-WebSocket-Protocol") != spec.SecWebSocketProtocol {
		warnAndClose(writer, "download: missing Sec-WebSocket-Protocol in request")
		return
	}
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	conn, err := dl.Upgrader.Upgrade(writer, request, headers)
	if err != nil {
		warnAndClose(writer, "download: cannnot UPGRADE to WebSocket")
		return
	}
	// TODO(bassosimone): an error before this point means that the *os.File
	// will stay in cache until the cache pruning mechanism is triggered. This
	// should be a small amount of seconds. If Golang does not call shutdown(2)
	// and close(2), we'll end up keeping sockets that caused an error in the
	// code above (e.g. because the handshake was not okay) alive for the time
	// in which the corresponding *os.File is kept in cache.
	defer warnonerror.Close(conn, "download: ignoring conn.Close result")
	logging.Logger.Debug("download: opening results file")
	resultfp, err := results.OpenFor(request, conn, dl.DataDir, "download")
	if err != nil {
		return // error already printed
	}
	defer warnonerror.Close(resultfp, "download: ignoring resultfp.Close result")
	// Implementation note: use child context so that, if we cannot save the
	// results in the loop below, we terminate the goroutines early
	wholectx, cancel := context.WithCancel(request.Context())
	defer cancel()
	senderch := sender.Start(conn, measurer.Start(wholectx, conn))
	receiverch := receiver.Start(wholectx, conn)
	zipch := zip(senderch, receiverch)
	defer func() {
		logging.Logger.Debug("download: start draining zipch")
		defer logging.Logger.Debug("download: stop draining zipch")
		for range zipch {
			// make sure we drain the channel if we leave the loop below early
			// because we cannot save some results
		}
	}()
	logging.Logger.Debug("download: start")
	defer logging.Logger.Debug("download: stop")
	for imsg := range zipch {
		if err := resultfp.WriteMeasurement(imsg.m, imsg.o); err != nil {
			logging.Logger.WithError(err).Warn(
				"download: resultfp.WriteMeasurement failed",
			)
			break
		}
	}
}
