package data

import (
	"time"

	"github.com/m-lab/ndt-server/ndt5/c2s"
	"github.com/m-lab/ndt-server/ndt5/control"
	"github.com/m-lab/ndt-server/ndt5/s2c"

	"github.com/m-lab/ndt-server/ndt7/model"
)

// NDT5Result is the struct that is serialized as JSON to disk as the archival
// record of an NDT test.
//
// This struct is dual-purpose. It contains the necessary data to allow joining
// with tcp-info data and traceroute-caller data as well as any other UUID-based
// data. It also contains enough data for interested parties to perform
// lightweight data analysis without needing to join with other tools.
//
// WARNING: The BigQuery schema is inferred directly from this structure. To
// preserve compatibility with historical data, never remove fields.
// For more information see: https://github.com/m-lab/etl/issues/719
type NDT5Result struct {
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
	Control *control.ArchivalData `json:",omitempty"`
	C2S     *c2s.ArchivalData     `json:",omitempty"`
	S2C     *s2c.ArchivalData     `json:",omitempty"`
}

// NDT7Result is the struct that is serialized as JSON to disk as the archival
// record of an NDT7 test. This is similar to, but independent from, the NDTResult.
type NDT7Result struct {
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

	// ndt7
	Upload   *model.ArchivalData `json:",omitempty"`
	Download *model.ArchivalData `json:",omitempty"`
}
