package results

import (
	"compress/gzip"
	"encoding/json"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-server/fdcache"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/ndt7/model"
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

	// Data contains metadata about the complete measurement.
	Data *model.ArchivalData
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
		Data: &model.ArchivalData{
			UUID: uuid,
		},
	}, nil
}

// OpenFor opens the results file and writes into it the results metadata based
// on the query string. Returns the results file on success. Returns an error in
// case of failure. The request argument is used to get the query string
// containing the metadata. The conn argument is used to retrieve the local and
// the remote endpoints addresses. The "datadir" argument specifies the
// directory on disk to write the data into and the what argument should
// indicate whether this is a spec.SubtestDownload or a spec.SubtestUpload
// ndt7 measurement.
func OpenFor(request *http.Request, conn *websocket.Conn, datadir string, what spec.SubtestKind) (*File, error) {
	meta := make(metadata, 0)
	netConn := conn.UnderlyingConn()
	id, err := fdcache.GetUUID(netConn)
	if err != nil {
		logging.Logger.WithError(err).Warn("fdcache.GetUUID failed")
		return nil, err
	}
	initMetadata(&meta, request.URL.Query())
	resultfp, err := newFile(datadir, string(what), id)
	if err != nil {
		logging.Logger.WithError(err).Warn("newFile failed")
		return nil, err
	}
	// Save client metadata.
	resultfp.SetMetadata(meta)
	return resultfp, nil
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

// AppendClientMeasurement saves the |measurement| for archival data.
func (fp *File) AppendClientMeasurement(measurement model.Measurement) {
	fp.Data.ClientMeasurements = append(fp.Data.ClientMeasurements, measurement)
}

// AppendServerMeasurement saves the |measurement| for archival data.
func (fp *File) AppendServerMeasurement(measurement model.Measurement) {
	fp.Data.ServerMeasurements = append(fp.Data.ServerMeasurements, measurement)
}

// SetMetadata writes |metadata| on the measurements file.
func (fp *File) SetMetadata(metadata metadata) {
	fp.Data.ClientMetadata = metadata
}

// StartTest records the test start time.
func (fp *File) StartTest() {
	fp.Data.StartTime = time.Now()
}

// EndTest records the test end time.
func (fp *File) EndTest() {
	fp.Data.EndTime = time.Now()
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
