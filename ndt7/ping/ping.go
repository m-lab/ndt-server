// Package ping implements the ndt7 ping test.
package ping

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/ndt7/measurer"
	"github.com/m-lab/ndt-server/ndt7/ping/mux"
	"github.com/m-lab/ndt-server/ndt7/ping/receiver"
	"github.com/m-lab/ndt-server/ndt7/ping/sender"
	"github.com/m-lab/ndt-server/ndt7/results"
	"github.com/m-lab/ndt-server/ndt7/saver"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

// Do implements the ping subtest. The ctx argument is the parent
// context for the subtest. The conn argument is the open WebSocket
// connection. The resultfp argument is the file where to save results. Both
// arguments are owned by the caller of this function. The start argument is
// the test start time used to calculate ElapsedTime and deadlines.
func Do(ctx context.Context, conn *websocket.Conn, resultfp *results.File, start time.Time) {
	wholectx, cancel := context.WithTimeout(ctx, spec.MaxRuntime)
	// saver.SaveAll() blocks till channels are drained, so cancel() is just for consistency here.
	defer cancel()
	measurerch := measurer.Start(wholectx, conn, resultfp.Data.UUID, start)
	receiverch := receiver.Start(wholectx, conn, start)
	x := mux.Start(measurerch, receiverch, cancel)
	sender.Start(conn, x.SenderC, start)
	saver.SaveAll(resultfp, x.ServerLog, x.ClientLog)
}
