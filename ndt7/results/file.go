package results

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path"
	"time"

	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/spec"
)

// File is the file where we save measurements.
type File struct {
	// Writer is the gzip writer instance
	Writer *gzip.Writer

	// Fp is the underlying file
	Fp *os.File

	// UUID is the UUID of this subtest
	UUID string
}

// newFile opens a measurements file in the current working
// directory on success and returns an error on failure.
func newFile(datadir, what, uuid string) (*File, error) {
	timestamp := time.Now().UTC()
	dir := path.Join(datadir, "ndt7", timestamp.Format("2006/01/02"))
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, err
	}
	name := dir + "/ndt7-" + what + "-" + timestamp.Format("20060102T150405.000000000Z") + "." + uuid + ".json.gz"
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
	return &File{
		Writer: writer,
		Fp:     fp,
	}, nil
}

// NewFile creates a file for saving results in datadir named after the uuid and
// kind. Returns the results file on success. Returns an error in case of
// failure. The "datadir" argument specifies the directory on disk to write the
// data into and the what argument should indicate whether this is a
// spec.SubtestDownload or a spec.SubtestUpload ndt7 measurement.
func NewFile(uuid string, datadir string, what spec.SubtestKind) (*File, error) {
	fp, err := newFile(datadir, string(what), uuid)
	if err != nil {
		logging.Logger.WithError(err).Warn("newFile failed")
		return nil, err
	}
	return fp, nil
}

// Close closes the measurement file.
func (fp *File) Close() error {
	err := fp.Writer.Close()
	if err != nil {
		fp.Fp.Close()
		return err
	}
	return fp.Fp.Close()
}

// WriteResult serializes |result| as JSON.
func (fp *File) WriteResult(result interface{}) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = fp.Writer.Write(data)
	return err
}
