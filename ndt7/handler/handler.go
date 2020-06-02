// Package handler implements the WebSocket handler for ndt7.
package handler

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/access/controller"
	"github.com/m-lab/go/prometheusx"
	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/data"
	"github.com/m-lab/ndt-server/fdcache"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/metadata"
	"github.com/m-lab/ndt-server/metrics"
	"github.com/m-lab/ndt-server/ndt7/download"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/results"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/ndt7/upload"
	"github.com/m-lab/ndt-server/version"
)

// Handler handles ndt7 subtests.
type Handler struct {
	// DataDir is the directory where results are saved.
	DataDir string
	// SecurePort should contain the port used for secure, WSS tests.
	SecurePort string
	// InsecurePort should contain the port used for insecure, WS tests.
	InsecurePort string
}

// warnAndClose emits message as a warning and the sends a Bad Request
// response to the client using writer.
func warnAndClose(writer http.ResponseWriter, message string) {
	logging.Logger.Warn(message)
	writer.Header().Set("Connection", "Close")
	writer.WriteHeader(http.StatusBadRequest)
}

// Download handles the download subtest.
func (h Handler) Download(rw http.ResponseWriter, req *http.Request) {
	h.runMeasurement(spec.SubtestDownload, rw, req)
}

// Upload handles the upload subtest.
func (h Handler) Upload(rw http.ResponseWriter, req *http.Request) {
	h.runMeasurement(spec.SubtestUpload, rw, req)
}

// runMeasurement conditionally runs either download or upload based on kind.
// The kind argument must be spec.SubtestDownload or spec.SubtestUpload.
func (h Handler) runMeasurement(kind spec.SubtestKind, rw http.ResponseWriter, req *http.Request) {
	// Setup websocket connection.
	conn := setupConn(rw, req)
	if conn == nil {
		// TODO: test failure.
		return
	}
	// Create measurement archival data.
	data, err := getData(conn)
	if err != nil {
		// TODO: test failure.
		return
	}
	// Collect most client metadata from request parameters.
	appendClientMetadata(data, req.URL.Query())
	// Create ultimate result.
	result := setupResult(conn)

	// Guarantee results are written even if function panics.
	defer func() {
		result.EndTime = time.Now().UTC()
		h.writeResult(data.UUID, kind, result)
	}()

	// Run measurement.
	if kind == spec.SubtestDownload {
		result.Download = data
		download.Do(req.Context(), conn, data)
		h.observe(conn, req, string(kind), downRate(data.ServerMeasurements))
	} else if kind == spec.SubtestUpload {
		result.Upload = data
		upload.Do(req.Context(), conn, data)
		h.observe(conn, req, string(kind), upRate(data.ServerMeasurements))
	}
}

// getProtocol infers an appropriate label for the websocket protocol.
func (h Handler) getProtocol(conn *websocket.Conn) string {
	if strings.HasSuffix(conn.LocalAddr().String(), h.SecurePort) {
		return "ndt7+wss"
	}
	if strings.HasSuffix(conn.LocalAddr().String(), h.InsecurePort) {
		return "ndt7+ws"
	}
	return "ndt7+unknown"
}

// setupConn negotiates a websocket connection. The writer argument is the HTTP
// response writer. The request argument is the HTTP request that we received.
func setupConn(writer http.ResponseWriter, request *http.Request) *websocket.Conn {
	logging.Logger.Debug("downloadOrUpload: upgrading to WebSockets")
	if request.Header.Get("Sec-WebSocket-Protocol") != spec.SecWebSocketProtocol {
		warnAndClose(
			writer, "downloadOrUpload: missing Sec-WebSocket-Protocol in request")
		return nil
	}
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow cross origin resource sharing
		},
		ReadBufferSize:  spec.MaxMessageSize,
		WriteBufferSize: spec.MaxMessageSize,
	}
	conn, err := upgrader.Upgrade(writer, request, headers)
	if err != nil {
		return nil
	}
	// TODO(bassosimone): an error before this point means that the *os.File
	// will stay in cache until the cache pruning mechanism is triggered. This
	// should be a small amount of seconds. If Golang does not call shutdown(2)
	// and close(2), we'll end up keeping sockets that caused an error in the
	// code above (e.g. because the handshake was not okay) alive for the time
	// in which the corresponding *os.File is kept in cache.
	defer warnonerror.Close(conn, "downloadOrUpload: ignoring conn.Close result")
	logging.Logger.Debug("downloadOrUpload: opening results file")

	return conn
}

// setupResult creates an NDT7Result from the given conn.
func setupResult(conn *websocket.Conn) *data.NDT7Result {
	// NOTE: unless we plan to run the NDT server over different protocols than TCP,
	// then we expect RemoteAddr and LocalAddr to always return net.TCPAddr types.
	clientAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		clientAddr = &net.TCPAddr{IP: net.ParseIP("::1"), Port: 1}
	}
	serverAddr, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		serverAddr = &net.TCPAddr{IP: net.ParseIP("::1"), Port: 1}
	}
	result := &data.NDT7Result{
		GitShortCommit: prometheusx.GitShortCommit,
		Version:        version.Version,
		ClientIP:       clientAddr.IP.String(),
		ClientPort:     clientAddr.Port,
		ServerIP:       serverAddr.IP.String(),
		ServerPort:     serverAddr.Port,
		StartTime:      time.Now(),
	}
	return result
}

func (h Handler) observe(conn *websocket.Conn, request *http.Request, direction string, val float64) {
	if val > 0 {
		isMon := fmt.Sprintf("%t", controller.IsMonitoring(controller.GetClaim(request.Context())))
		proto := h.getProtocol(conn)
		// Update the download rates histogram.
		metrics.TestRate.WithLabelValues(proto, direction, isMon).Observe(val)
	}
}

func (h Handler) writeResult(uuid string, kind spec.SubtestKind, result *data.NDT7Result) {
	fp, err := results.NewFile(uuid, h.DataDir, kind)
	if err != nil {
		logging.Logger.WithError(err).Warn("results.NewFile failed")
		return
	}
	result.EndTime = time.Now().UTC()
	if err := fp.WriteResult(result); err != nil {
		logging.Logger.WithError(err).Warn("failed to write result")
	}
	warnonerror.Close(fp, string(kind)+": ignoring fp.Close error")
}

func getData(conn *websocket.Conn) (*model.ArchivalData, error) {
	// TODO(m-lab/ndt-server/issues/235): delete the fdcache.
	netConn := conn.UnderlyingConn()
	uuid, err := fdcache.GetUUID(netConn)
	if err != nil {
		logging.Logger.WithError(err).Warn("fdcache.GetUUID failed")
		return nil, err
	}
	data := &model.ArchivalData{
		UUID: uuid,
	}
	return data, nil
}

func upRate(m []model.Measurement) float64 {
	var mbps float64
	if len(m) > 0 {
		// Convert to Mbps.
		mbps = 8 * float64(m[len(m)-1].TCPInfo.BytesReceived) / float64(m[len(m)-1].TCPInfo.ElapsedTime)
	}
	return mbps
}

func downRate(m []model.Measurement) float64 {
	var mbps float64
	if len(m) > 0 {
		// Convert to Mbps.
		mbps = 8 * float64(m[len(m)-1].TCPInfo.BytesAcked) / float64(m[len(m)-1].TCPInfo.ElapsedTime)
	}
	return mbps
}

// excludeKeyRe is a regexp for excluding request parameters from client metadata.
var excludeKeyRe = regexp.MustCompile("^server_")

// appendClientMetadata adds |values| to the archival client metadata contained
// in the request parameter values. Some select key patterns will be excluded.
func appendClientMetadata(data *model.ArchivalData, values url.Values) {
	for name, values := range values {
		if matches := excludeKeyRe.MatchString(name); matches {
			continue // Skip variables that should be excluded.
		}
		data.ClientMetadata = append(
			data.ClientMetadata,
			metadata.NameValue{
				Name:  name,
				Value: values[0], // NOTE: this will ignore multi-value parameters.
			})
	}
}
