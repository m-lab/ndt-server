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
	"github.com/m-lab/ndt-server/ndt7/model"
)

// File is the file where we save measurements.
type File struct {
	// Writer is the gzip writer instance
	Writer *gzip.Writer
	// Fp is the underlying file
	Fp *os.File
}

// newFile opens a measurements file in the current working
// directory on success and returns an error on failure.
func newFile(datadir, what, uuid string) (*File, error) {
	timestamp := time.Now().UTC()
	dir := path.Join(datadir, what, timestamp.Format("2006/01/02"))
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, err
	}
	name := dir + "/ndt7-" + timestamp.Format("2006-01-02T15:04:05.000000000Z") + "." + uuid + ".jsonl.gz"
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

// OpenFor opens the results file and writes into it the results metadata based
// on the query string. Returns the results file on success. Returns an error in
// case of failure. The request argument is used to get the query string
// containing the metadata. The conn argument is used to retrieve the local and
// the remote endpoints addresses. The "datadir" argument specifies the
// directory on disk to write the data into and the what argument should
// indicate whether this is a "download" or an "upload" ndt7 measurement.
func OpenFor(request *http.Request, conn *websocket.Conn, datadir, what string) (*File, error) {
	meta := make(metadata)
	netConn := conn.UnderlyingConn()
	id, err := fdcache.GetUUID(netConn)
	if err != nil {
		return nil, err
	}
	initMetadata(&meta, conn.LocalAddr().String(), conn.RemoteAddr().String(), id,
		request.URL.Query(), what)
	resultfp, err := newFile(datadir, what, id)
	if err != nil {
		return nil, err
	}
	if err := resultfp.WriteMetadata(meta); err != nil {
		resultfp.Close()
		return nil, err
	}
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

// savedMeasurement is a saved measurement.
type savedMeasurement struct {
	// Origin is either "client" or "server" depending on who performed
	// the measurement that is currently being saved.
	Origin string `json:"origin"`
	// Measurement is the actual measurement to be saved.
	Measurement model.Measurement `json:"measurement"`
}

// WriteMeasurement writes |measurement| on the measurements file.
func (fp *File) WriteMeasurement(measurement model.Measurement, origin string) error {
	return fp.writeInterface(savedMeasurement{
		Origin:      origin,
		Measurement: measurement,
	})
}

// WriteMetadata writes |metadata| on the measurements file.
func (fp *File) WriteMetadata(metadata metadata) error {
	return fp.writeInterface(metadata)
}

// writeInterface serializes |entry| as JSONL.
func (fp *File) writeInterface(entry interface{}) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, byte('\n'))
	_, err = fp.Writer.Write(data)
	return err
}
