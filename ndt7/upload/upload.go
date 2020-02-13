// Package upload implements the ndt7 upload
package upload

import (
	"context"
	"time"

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
// owned by the caller of this function.  The start argument is the test
// start time used to calculate ElapsedTime and deadlines.
func Do(ctx context.Context, conn *websocket.Conn, resultfp *results.File, start time.Time) {
	// Implementation note: use child context so that, if we cannot save the
	// results in the loop below, we terminate the goroutines early
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	measurer := measurer.New(conn, resultfp.Data.UUID, start)
	senderch := sender.Start(conn, measurer.Start(ctx), start)
	receiverch := receiver.StartUploadReceiver(ctx, conn, start)
	saver.SaveAll(resultfp, senderch, receiverch)
}
