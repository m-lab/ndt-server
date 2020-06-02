// Package download implements the ndt7/server downloader.
package download

import (
	"context"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/ndt7/download/sender"
	"github.com/m-lab/ndt-server/ndt7/measurer"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/receiver"
	"github.com/m-lab/ndt-server/ndt7/saver"
)

// Do implements the download subtest. The ctx argument is the parent context
// for the subtest. The conn argument is the open WebSocket connection. The data
// argument is the archival data where results are saved. Both arguments are
// owned by the caller of this function.
func Do(ctx context.Context, conn *websocket.Conn, data *model.ArchivalData) {
	// Implementation note: use child context so that, if we cannot save the
	// results in the loop below, we terminate the goroutines early
	wholectx, cancel := context.WithCancel(ctx)
	defer cancel()
	measurer := measurer.New(conn, data.UUID)
	senderch := sender.Start(conn, measurer.Start(ctx))
	receiverch := receiver.StartDownloadReceiver(wholectx, conn)
	saver.SaveAll(data, senderch, receiverch)
}
