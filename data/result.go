package data

import (
	"time"

	"github.com/m-lab/ndt-server/ndt5/c2s"
	"github.com/m-lab/ndt-server/ndt5/control"
	"github.com/m-lab/ndt-server/ndt5/meta"
	"github.com/m-lab/ndt-server/ndt5/s2c"

	"github.com/m-lab/ndt-server/ndt7/download"
	"github.com/m-lab/ndt-server/ndt7/upload"
)

// NDTResult is the struct that is serialized as JSON to disk as the archival
// record of an NDT test.
//
// This struct is dual-purpose. It contains the necessary data to allow joining
// with tcp-info data and traceroute-caller data as well as any other UUID-based
// data. It also contains enough data for interested parties to perform
// lightweight data analysis without needing to join with other tools.
type NDTResult struct {
	// GitShortCommit is the Git commit (short form) of the running server code.
	GitShortCommit string
	// Version is the symbolic version (if any) of the running server code.
	Version string

	// All data members should all be self-describing. In the event of confusion,
	// rename them to add clarity rather than adding a comment.
	ServerIP   string
	ServerPort int
	ClientIP   string
	ClientPort int

	StartTime time.Time
	EndTime   time.Time

	// ndt5
	Control        *control.ArchivalData `json:",omitempty"`
	C2S            *c2s.ArchivalData     `json:",omitempty"`
	S2C            *s2c.ArchivalData     `json:",omitempty"`
	ClientMetadata meta.ArchivalData     `json:",omitempty"`

	// ndt7
	Upload   *upload.ArchivalData   `json:",omitempty"`
	Download *download.ArchivalData `json:",omitempty"`
}
