package data

import (
	"time"

	"github.com/m-lab/ndt-server/ndt5/c2s"
	"github.com/m-lab/ndt-server/ndt5/control"
	"github.com/m-lab/ndt-server/ndt5/s2c"

	"github.com/m-lab/ndt-server/ndt7/model"
)

// CurrentSchemaVersion is the current version of the NDTResult struct below.
// This schema version should be included in serialized JSON result files. The
// version should be incremented for every structure change to NDTResult so
// that the mirror structures in the ETL parser can be updated.
const CurrentSchemaVersion = 1

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
	// SchemaVersion represents the version of the NDTResult structure. This is
	// needed to track evolving changes to the structure over time and keep all
	// historical data parsable by the ETL parser.
	SchemaVersion int

	// All data members should all be self-describing. In the event of confusion,
	// rename them to add clarity rather than adding a comment.
	ServerIP   string
	ServerPort int
	ClientIP   string
	ClientPort int

	StartTime time.Time
	EndTime   time.Time

	// ndt5
	Control *control.ArchivalData `json:",omitempty"`
	C2S     *c2s.ArchivalData     `json:",omitempty"`
	S2C     *s2c.ArchivalData     `json:",omitempty"`

	// ndt7
	Upload   *model.ArchivalData `json:",omitempty"`
	Download *model.ArchivalData `json:",omitempty"`
}
