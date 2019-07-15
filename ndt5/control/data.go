package control

import "github.com/m-lab/ndt-server/ndt5/ndt"

type ArchivalData struct {
	// These data members should all be self-describing. In the event of confusion,
	// rename them to add clarity rather than adding a comment.
	ChannelUUID     string
	Protocol        ndt.ConnectionType
	MessageProtocol string
}
