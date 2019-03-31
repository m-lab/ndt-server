// Package download implements the ndt7/server downloader.
package download

import (
	"context"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/server/download/measurer"
	"github.com/m-lab/ndt-server/ndt7/server/download/receiver"
	"github.com/m-lab/ndt-server/ndt7/server/download/sender"
	"github.com/m-lab/ndt-server/ndt7/server/results"
	"github.com/m-lab/ndt-server/ndt7/server/saver"
)

// Do performs the download subtest. Note that this function does not own
// conn and resultsfp, which are still owned by the caller.
func Do(ctx context.Context, conn *websocket.Conn, resultsfp *results.File) {
	senderch := sender.Start(conn, measurer.Start(ctx, conn))
	receiverch := receiver.Start(ctx, conn)
	logging.Logger.Debug("download: start")
	defer logging.Logger.Debug("download: stop")
	saver.SaveAll(
		senderch, // during download sender performs server-side measurements
		receiverch, // likewise receiver gets measurements from the client
		resultsfp,
	)
}
