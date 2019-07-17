package model

import "time"

// ArchivalData saves all instantaneous measurements over the lifetime of a test.
type ArchivalData struct {
	UUID               string
	StartTime          time.Time
	EndTime            time.Time
	ServerMeasurements []Measurement
	ClientMeasurements []Measurement
	// TODO(m-lab/ndt-server/issues/151): remove bigquery tag.
	ClientMetadata map[string]string `json:",omitempty" bigquery:"-"`
}
