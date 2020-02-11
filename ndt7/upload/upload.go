// Package upload implements the ndt7 upload
package upload

import (
	"context"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/ndt7/measurer"
	"github.com/m-lab/ndt-server/ndt7/receiver"
	"github.com/m-lab/ndt-server/ndt7/results"
	"github.com/m-lab/ndt-server/ndt7/saver"
	"github.com/m-lab/ndt-server/ndt7/upload/sender"
)

// Do implements the upload subtest. The ctx argument is the parent context
// for the subtest. The conn argument is the open WebSocket connection. The
// resultfp argument is the file where to save results. Both arguments are
// owned by the caller of this function.
func Do(ctx context.Context, conn *websocket.Conn, resultfp *results.File) {
	// Implementation note: use child context so that, if we cannot save the
	// results in the loop below, we terminate the goroutines early
	wholectx, cancel := context.WithCancel(ctx)
	defer cancel()
	measurer := measurer.New(conn, resultfp.Data.UUID)
	senderch := sender.Start(conn, measurer.Start(ctx))
	receiverch := receiver.StartUploadReceiver(wholectx, conn)
	saver.SaveAll(resultfp, senderch, receiverch)
}
