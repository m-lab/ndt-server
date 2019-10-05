package model

import (
	"time"

	"github.com/m-lab/ndt-server/metadata"
)

// ArchivalData saves all instantaneous measurements over the lifetime of a test.
type ArchivalData struct {
	UUID               string
	StartTime          time.Time
	EndTime            time.Time
	ServerMeasurements []Measurement
	ClientMeasurements []Measurement
	ClientMetadata     []metadata.NameValue `json:",omitempty"`
}
