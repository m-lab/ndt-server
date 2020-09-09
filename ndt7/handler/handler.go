// Package handler implements the WebSocket handler for ndt7.
package handler

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/gorilla/websocket"

	"github.com/m-lab/access/controller"
	"github.com/m-lab/go/prometheusx"
	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/data"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/metadata"
	"github.com/m-lab/ndt-server/metrics"
	"github.com/m-lab/ndt-server/ndt7/download"
	ndt7metrics "github.com/m-lab/ndt-server/ndt7/metrics"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/results"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/ndt7/upload"
	"github.com/m-lab/ndt-server/netx"
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
		ndt7metrics.ClientConnections.WithLabelValues(string(kind), "websocket-error").Inc()
		return
	}
	defer warnonerror.Close(conn, "runMeasurement: ignoring conn.Close result")
	// Create measurement archival data.
	data, err := getData(conn)
	if err != nil {
		// TODO: test failure.
		ndt7metrics.ClientConnections.WithLabelValues(string(kind), "uuid-error").Inc()
		return
	}
	// We are guaranteed to collect a result at this point (even if it's with an error)
	ndt7metrics.ClientConnections.WithLabelValues(string(kind), "result").Inc()

	// Collect most client metadata from request parameters.
	appendClientMetadata(data, req.URL.Query())
	// Create ultimate result.
	result := setupResult(conn)
	result.StartTime = time.Now().UTC()

	// Guarantee results are written even if function panics.
	defer func() {
		result.EndTime = time.Now().UTC()
		h.writeResult(data.UUID, kind, result)
	}()

	// Run measurement.
	var rate float64
	if kind == spec.SubtestDownload {
		result.Download = data
		err = download.Do(req.Context(), conn, data)
		rate = downRate(data.ServerMeasurements)
	} else if kind == spec.SubtestUpload {
		result.Upload = data
		err = upload.Do(req.Context(), conn, data)
		rate = upRate(data.ServerMeasurements)
	}

	proto := ndt7metrics.ConnLabel(conn)
	ndt7metrics.ClientTestResults.WithLabelValues(
		proto, string(kind), metrics.GetResultLabel(err, rate)).Inc()
	if rate > 0 {
		isMon := fmt.Sprintf("%t", controller.IsMonitoring(controller.GetClaim(req.Context())))
		// Update the common (ndt5+ndt7) measurement rates histogram.
		metrics.TestRate.WithLabelValues(proto, string(kind), isMon).Observe(rate)
	}
}

// setupConn negotiates a websocket connection. The writer argument is the HTTP
// response writer. The request argument is the HTTP request that we received.
func setupConn(writer http.ResponseWriter, request *http.Request) *websocket.Conn {
	logging.Logger.Debug("setupConn: upgrading to WebSockets")
	if request.Header.Get("Sec-WebSocket-Protocol") != spec.SecWebSocketProtocol {
		warnAndClose(
			writer, "setupConn: missing Sec-WebSocket-Protocol in request")
		return nil
	}
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow cross origin resource sharing
		},
		ReadBufferSize:  spec.DefaultWebsocketBufferSize,
		WriteBufferSize: spec.DefaultWebsocketBufferSize,
	}
	conn, err := upgrader.Upgrade(writer, request, headers)
	if err != nil {
		return nil
	}
	logging.Logger.Debug("setupConn: opening results file")

	return conn
}

// setupResult creates an NDT7Result from the given conn.
func setupResult(conn *websocket.Conn) *data.NDT7Result {
	// NOTE: unless we plan to run the NDT server over different protocols than TCP,
	// then we expect RemoteAddr and LocalAddr to always return net.TCPAddr types.
	clientAddr := netx.ToTCPAddr(conn.RemoteAddr())
	if clientAddr == nil {
		clientAddr = &net.TCPAddr{IP: net.ParseIP("::1"), Port: 1}
	}
	serverAddr := netx.ToTCPAddr(conn.LocalAddr())
	if serverAddr == nil {
		serverAddr = &net.TCPAddr{IP: net.ParseIP("::1"), Port: 1}
	}
	result := &data.NDT7Result{
		GitShortCommit: prometheusx.GitShortCommit,
		Version:        version.Version,
		ClientIP:       clientAddr.IP.String(),
		ClientPort:     clientAddr.Port,
		ServerIP:       serverAddr.IP.String(),
		ServerPort:     serverAddr.Port,
	}
	return result
}

func (h Handler) writeResult(uuid string, kind spec.SubtestKind, result *data.NDT7Result) {
	fp, err := results.NewFile(uuid, h.DataDir, kind)
	if err != nil {
		logging.Logger.WithError(err).Warn("results.NewFile failed")
		return
	}
	if err := fp.WriteResult(result); err != nil {
		logging.Logger.WithError(err).Warn("failed to write result")
	}
	warnonerror.Close(fp, string(kind)+": ignoring fp.Close error")
}

func getData(conn *websocket.Conn) (*model.ArchivalData, error) {
	ci := netx.ToConnInfo(conn.UnderlyingConn())
	uuid, err := ci.GetUUID()
	if err != nil {
		logging.Logger.WithError(err).Warn("conninfo.GetUUID failed")
		return nil, err
	}
	data := &model.ArchivalData{
		UUID: uuid,
	}
	return data, nil
}

func upRate(m []model.Measurement) float64 {
	var mbps float64
	// NOTE: on non-Linux platforms, TCPInfo will be nil.
	if len(m) > 0 && m[len(m)-1].TCPInfo != nil {
		// Convert to Mbps.
		mbps = 8 * float64(m[len(m)-1].TCPInfo.BytesReceived) / float64(m[len(m)-1].TCPInfo.ElapsedTime)
	}
	return mbps
}

func downRate(m []model.Measurement) float64 {
	var mbps float64
	// NOTE: on non-Linux platforms, TCPInfo will be nil.
	if len(m) > 0 && m[len(m)-1].TCPInfo != nil {
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
