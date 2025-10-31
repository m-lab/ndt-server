// Package handler implements the WebSocket handler for ndt7.
package handler

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/exp/slices"

	"github.com/m-lab/access/controller"
	"github.com/m-lab/go/prometheusx"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/data"
	"github.com/m-lab/ndt-server/logging"
	"github.com/m-lab/ndt-server/metadata"
	"github.com/m-lab/ndt-server/metrics"
	"github.com/m-lab/ndt-server/ndt7/download"
	"github.com/m-lab/ndt-server/ndt7/download/sender"
	ndt7metrics "github.com/m-lab/ndt-server/ndt7/metrics"
	"github.com/m-lab/ndt-server/ndt7/model"
	"github.com/m-lab/ndt-server/ndt7/results"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/ndt7/upload"
	"github.com/m-lab/ndt-server/netx"
	"github.com/m-lab/ndt-server/redis"
	"github.com/m-lab/ndt-server/version"
	"github.com/m-lab/tcp-info/eventsocket"
	"github.com/m-lab/tcp-info/inetdiag"
)

// Handler handles ndt7 subtests.
type Handler struct {
	// DataDir is the directory where results are saved.
	DataDir string
	// SecurePort should contain the port used for secure, WSS tests.
	SecurePort string
	// InsecurePort should contain the port used for insecure, WS tests.
	InsecurePort string
	// ServerMetadata contains deployment-specific metadata.
	ServerMetadata []metadata.NameValue
	// CompressResults controls whether the result files saved by the server are compressed.
	CompressResults bool
	// Events is for reporting new connections to the event server.
	Events eventsocket.Server
	// RedisClient is the Redis client for caching.
	RedisClient *redis.Client
}

// warnAndClose emits message as a warning and the sends a Bad Request
// response to the client using writer.
func warnAndClose(writer http.ResponseWriter, message string) {
	logging.Logger.Warn(message)
	writer.Header().Set("Connection", "Close")
	writer.WriteHeader(http.StatusBadRequest)
}

// Download handles the download subtest.
func (h *Handler) Download(rw http.ResponseWriter, req *http.Request) {
	h.runMeasurement(spec.SubtestDownload, rw, req)
}

// Upload handles the upload subtest.
func (h *Handler) Upload(rw http.ResponseWriter, req *http.Request) {
	h.runMeasurement(spec.SubtestUpload, rw, req)
}

// runMeasurement conditionally runs either download or upload based on kind.
// The kind argument must be spec.SubtestDownload or spec.SubtestUpload.
func (h *Handler) runMeasurement(kind spec.SubtestKind, rw http.ResponseWriter, req *http.Request) {
	// Validate client request before opening the connection.
	params, err := validateEarlyExit(req.URL.Query())
	if err != nil {
		warnAndClose(rw, err.Error())
		return
	}

	// Setup websocket connection.
	conn := setupConn(rw, req)
	if conn == nil {
		// TODO: test failure.
		ndt7metrics.ClientConnections.WithLabelValues(string(kind), "websocket-error").Inc()
		return
	}
	// Make sure that the connection is closed after (at most) MaxRuntime.
	// Download and upload tests have their own timeouts, but we have observed
	// that under particular network conditions the connection can remain open
	// while the receiver goroutine is blocked on a read syscall, long after
	// the client is gone. This is a workaround for that.
	ctx, cancel := context.WithTimeout(req.Context(), spec.MaxRuntime)
	defer cancel()
	go func() {
		<-ctx.Done()
		warnonerror.Close(conn, "runMeasurement: ignoring conn.Close result")
	}()
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
	data.ServerMetadata = h.ServerMetadata
	// Create ultimate result.
	result, id := setupResult(conn)
	result.StartTime = time.Now().UTC()
	h.Events.FlowCreated(result.StartTime, data.UUID, id)

	// Guarantee results are written even if subtest functions panic.
	defer func() {
		result.EndTime = time.Now().UTC()
		h.writeResult(data.UUID, kind, result)
		h.Events.FlowDeleted(result.EndTime, data.UUID)
	}()

	// Run measurement.
	var rate float64
	if kind == spec.SubtestDownload {
		result.Download = data
		err = download.Do(ctx, conn, data, params)
		rate = downRate(data.ServerMeasurements)
	} else if kind == spec.SubtestUpload {
		result.Upload = data
		err = upload.Do(ctx, conn, data)
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
func setupResult(conn *websocket.Conn) (*data.NDT7Result, inetdiag.SockID) {
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
	id := inetdiag.SockID{
		SrcIP:  result.ServerIP,
		DstIP:  result.ClientIP,
		SPort:  uint16(result.ServerPort),
		DPort:  uint16(result.ClientPort),
		Cookie: -1, // Note: we do not populate the socket cookie here.
	}
	return result, id
}

func (h Handler) writeResult(uuid string, kind spec.SubtestKind, result *data.NDT7Result) {
	fp, err := results.NewFile(uuid, h.DataDir, kind, h.CompressResults)
	// Note: an ndt-server instance that cannot write results is not useful. This
	// is a fatal error.
	rtx.Must(err, "results.NewFile failed")
	err = fp.WriteResult(result)
	rtx.Must(err, "failed to write result")
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

// validateEarlyExit verifies and returns the "early_exit" parameters.
func validateEarlyExit(values url.Values) (*sender.Params, error) {
	for name, values := range values {
		if name != spec.EarlyExitParameterName {
			continue
		}

		value := values[0]
		if !slices.Contains(spec.ValidEarlyExitValues, value) {
			return nil, fmt.Errorf("invalid %s parameter value %s", name, value)
		}

		// Convert string to int64.
		bytes, _ := strconv.ParseInt(value, 10, 64)
		return &sender.Params{
			IsEarlyExit: true,
			MaxBytes:    bytes * 1000000, // Conver MB to bytes.
		}, nil
	}
	return &sender.Params{
		IsEarlyExit: false,
	}, nil
}

// checkEarlyTermination queries in-memory database for termination decision every 100 ms.
// It monitors the termination flag for the given UUID and calls the cancel function
// when early termination is requested.
//
//	0 is proceed (do not terminate early), 1 is terminate early
//	keys are prefixed by "table_2"
//	key: "table_2:uuid"
func (h *Handler) checkEarlyTermination(ctx context.Context, uuid string, cancel context.CancelFunc) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Context cancelled, stop monitoring
			return
		case <-ticker.C:
			// Query table_2 for termination decision
			flag, err := h.RedisClient.GetTerminationFlag(ctx, uuid)
			if err != nil {
				// Log error but continue monitoring
				logging.Logger.WithError(err).Warn("checkEarlyTermination: failed to get termination flag")
				continue
			}

			// If flag == 1, trigger early termination
			if flag == 1 {
				logging.Logger.Debug("checkEarlyTermination: termination flag set, cancelling context")
				cancel()
				return
			}
		}
	}
}
