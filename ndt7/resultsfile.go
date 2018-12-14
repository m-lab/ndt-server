package ndt7

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"time"

	"github.com/m-lab/ndt-cloud/ndt7/model"
)

// resultsfile is the file where we save measurements.
type resultsfile struct {
	// Writer is the gzip writer instance
	Writer *gzip.Writer
	// Fp is the underlying file
	Fp     *os.File
}

// newResultsfile opens a measurements file in the current working
// directory on success and returns an error on failure.
func newResultsfile() (*resultsfile, error) {
	format := "2006-01-02T15:04:05.000000000Z"
	timestamp := time.Now().UTC().Format(format)
	name := "ndt7-" + timestamp + ".jsonl.gz"
	// My assumption here is that we have nanosecond precision and hence it's
	// unlikely to have conflicts. If I'm wrong, O_EXCL will let us know.
	fp, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, err
	}
	writer, err := gzip.NewWriterLevel(fp, gzip.BestSpeed)
	if err != nil {
		fp.Close()
		return nil, err
	}
	return &resultsfile{
		Writer: writer,
		Fp: fp,
	}, nil
}

// Close closes the measurement file.
func (fp *resultsfile) Close() error {
	err := fp.Writer.Close()
	if err != nil {
		fp.Fp.Close()
		return err
	}
	return fp.Fp.Close()
}

// savedMeasurement is a saved measurement.
type savedMeasurement struct {
	// Origin is either "client" or "server" depending on who performed
	// the measurement that is currently being saved.
	Origin string `json:"origin"`
	// Measurement is the actual measurement to be saved.
	Measurement model.Measurement `json:"measurement"`
}

// WriteMeasurement writes |measurement| on the measurements file.
func (fp *resultsfile) WriteMeasurement(measurement model.Measurement, origin string) error {
	return fp.writeInterface(savedMeasurement{
		Origin: origin,
		Measurement: measurement,
	})
}

// WriteMetadata writes |metadata| on the measurements file.
func (fp *resultsfile) WriteMetadata(metadata metadata) error {
	return fp.writeInterface(metadata)
}

// writeInterface serializes |entry| as JSONL.
func (fp *resultsfile) writeInterface(entry interface{}) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, byte('\n'))
	_, err = fp.Writer.Write(data)
	return err
}
