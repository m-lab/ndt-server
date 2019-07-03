package ndt5

import "github.com/m-lab/ndt-server/ndt5/ndt"

type Metadata struct {
	// These data members should all be self-describing. In the event of confusion,
	// rename them to add clarity rather than adding a comment.
	ControlChannelUUID string
	Protocol           ndt.ConnectionType
	MessageProtocol    string
}
