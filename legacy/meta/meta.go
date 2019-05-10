package meta

import (
	"log"

	"github.com/m-lab/ndt-server/legacy/protocol"
)

// TODO: Add fields here when we implement a meta test.
type ArchivalData struct{}

// TODO: run meta test.
func ManageTest(ws protocol.Connection) {
	var err error
	var message *protocol.JSONMessage

	protocol.SendJSONMessage(protocol.TestPrepare, "", ws)
	protocol.SendJSONMessage(protocol.TestStart, "", ws)
	for {
		message, err = protocol.ReceiveJSONMessage(ws, protocol.TestMsg)
		if message.Msg == "" || err != nil {
			break
		}
		log.Println("Meta message: ", message)
	}
	if err != nil {
		log.Println("Error reading JSON message:", err)
		return
	}
	protocol.SendJSONMessage(protocol.TestFinalize, "", ws)
}
