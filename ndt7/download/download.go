// Package download implements the ndt7/server downloader.
package download

import (
	"context"
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/ndt7/download/sender"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/receiver"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

// Do implements the download subtest. The ctx argument is the parent context
// for the subtest. The conn argument is the open WebSocket connection. The data
// argument is the archival data where results are saved. Both arguments are
// owned by the caller of this function.
func Do(ctx context.Context, conn *websocket.Conn, data *model.ArchivalData) {
	// Implementation note: use child contexts so that, if we cannot save the
	// results in the loop below, we terminate the goroutines early
	fmt.Println("download start")

	// Receive and save client-provided measurements in data.
	rcvctx, rcvCancel := context.WithCancel(ctx)
	defer rcvCancel()
	recvDone := make(chan bool)
	go func() {
		receiver.StartDownloadReceiver(rcvctx, conn, data)
		close(recvDone)
	}()

	// Perform download and save server-measurements in data.
	dlctx, dlCancel := context.WithTimeout(ctx, spec.DefaultRuntime)
	defer dlCancel()
	// TODO: move sender.Start logic to this file.
	sender.Start(dlctx, conn, data)
	<-recvDone
	fmt.Println("download done")
}
