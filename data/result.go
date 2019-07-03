package data

import (
	"time"

	"github.com/m-lab/ndt-server/ndt5/c2s"
	ndt5data "github.com/m-lab/ndt-server/ndt5/data"
	"github.com/m-lab/ndt-server/ndt5/meta"
	"github.com/m-lab/ndt-server/ndt5/s2c"
)

// NDTResult is the struct that is serialized as JSON to disk as the archival record of an NDT test.
//
// This struct is dual-purpose. It contains the necessary data to allow joining
// with tcp-info data and traceroute-caller data as well as any other UUID-based
// data. It also contains enough data for interested parties to perform
// lightweight data analysis without needing to join with other tools.
type NDTResult struct {
	// GitShortCommit is the Git commit (short form) of the running server code.
	GitShortCommit string

	// All data members should all be self-describing. In the event of confusion,
	// rename them to add clarity rather than adding a comment.
	NDT5Metadata *ndt5data.Metadata `json:",omitempty"`

	ServerIP   string
	ServerPort int
	ClientIP   string
	ClientPort int

	StartTime time.Time
	EndTime   time.Time
	C2S       *c2s.ArchivalData `json:",omitempty"`
	S2C       *s2c.ArchivalData `json:",omitempty"`
	Meta      meta.ArchivalData `json:",omitempty"`
}
