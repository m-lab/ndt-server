// Package download implements the ndt7/server downloader.
package download

import (
	"context"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/ndt7/download/measurer"
	"github.com/m-lab/ndt-server/ndt7/download/receiver"
	"github.com/m-lab/ndt-server/ndt7/download/sender"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/results"
	"github.com/m-lab/ndt-server/ndt7/saver"
)

// ArchivalData saves all download test measurements.
type ArchivalData struct {
	UUID           string
	Measurements   []model.Measurement
	ClientMetadata map[string]string
}

// Do implements the download subtest. The ctx argument is the parent
// context for the subtest. The conn argument is the open WebSocket
// connection. The resultfp argument is the file where to save results. Both
// arguments are owned by the caller of this function.
func Do(ctx context.Context, conn *websocket.Conn, resultfp *results.File) {
	// Implementation note: use child context so that, if we cannot save the
	// results in the loop below, we terminate the goroutines early
	wholectx, cancel := context.WithCancel(ctx)
	defer cancel()
	senderch := sender.Start(conn, measurer.Start(wholectx, conn, resultfp))
	receiverch := receiver.Start(wholectx, conn)
	saver.SaveAll(resultfp, senderch, receiverch)
}
