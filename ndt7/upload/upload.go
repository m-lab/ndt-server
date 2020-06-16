// Package upload implements the ndt7 upload
package upload

import (
	"context"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/receiver"
	"github.com/m-lab/ndt-server/ndt7/upload/sender"
)

// Do implements the upload subtest. The ctx argument is the parent context for
// the subtest. The conn argument is the open WebSocket connection. The data
// argument is the archival data where results are saved. All arguments are
// owned by the caller of this function.
func Do(ctx context.Context, conn *websocket.Conn, data *model.ArchivalData) error {
	// Implementation note: use child contexts so the sender is strictly time
	// bounded. After timeout, the sender closes the conn, which results in the
	// receiver completing.

	// Receive and save client-provided measurements in data.
	recv := receiver.StartUploadReceiverAsync(ctx, conn, data)

	// Perform upload and save server-measurements in data.
	// TODO: move sender.Start logic to this file.
	err := sender.Start(ctx, conn, data)

	// Block on the receiver completing to guarantee that access to data is synchronous.
	<-recv.Done()
	return err
}
