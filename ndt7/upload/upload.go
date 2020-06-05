// Package upload implements the ndt7 upload
package upload

import (
	"context"
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/receiver"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/ndt7/upload/sender"
)

// Do implements the upload subtest. The ctx argument is the parent context
// for the subtest. The conn argument is the open WebSocket connection. The
// resultfp argument is the file where to save results. Both arguments are
// owned by the caller of this function.
func Do(ctx context.Context, conn *websocket.Conn, data *model.ArchivalData) {
	// Implementation note: use child context so that, if we cannot save the
	// results in the loop below, we terminate the goroutines early
	fmt.Println("upload start")

	// Receive and save client-provided measurements in data.
	rcvctx, rcvCancel := context.WithCancel(ctx)
	defer rcvCancel()
	recv := receiver.StartUploadReceiverAsync(rcvctx, conn, data)

	// Perform upload and save server-measurements in data.
	ulctx, ulCancel := context.WithTimeout(ctx, spec.DefaultRuntime)
	defer ulCancel()
	// TODO: move sender.Start logic to this file.
	sender.Start(ulctx, conn, data)
	<-recv.Done()
	fmt.Println("upload done")
}
