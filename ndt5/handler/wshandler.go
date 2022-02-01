package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/m-lab/access/controller"
	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/metadata"
	"github.com/m-lab/ndt-server/ndt5"
	"github.com/m-lab/ndt-server/ndt5/ndt"
	"github.com/m-lab/ndt-server/ndt5/protocol"
	"github.com/m-lab/ndt-server/ndt5/singleserving"
	"github.com/m-lab/ndt-server/ndt5/ws"
)

// WSHandler is both an ndt.Server and an http.Handler to allow websocket-based
// NDT tests to be run by Go's http libraries.
type WSHandler interface {
	ndt.Server
	http.Handler
}

type httpFactory struct{}

func (hf *httpFactory) SingleServingServer(dir string) (ndt.SingleMeasurementServer, error) {
	return singleserving.ListenWS(dir)
}

// httpHandler handles requests that come in over HTTP or HTTPS. It should be
// created with MakeHTTPHandler() or MakeHTTPSHandler().
type httpHandler struct {
	serverFactory  ndt.SingleMeasurementServerFactory
	connectionType ndt.ConnectionType
	datadir        string
	metadata       []metadata.NameValue
}

func (s *httpHandler) DataDir() string                    { return s.datadir }
func (s *httpHandler) ConnectionType() ndt.ConnectionType { return s.connectionType }
func (s *httpHandler) Metadata() []metadata.NameValue     { return s.metadata }

func (s *httpHandler) LoginCeremony(conn protocol.Connection) (int, error) {
	// WS and WSS both only support JSON clients and not TLV clients.
	msg, err := protocol.ReceiveJSONMessage(conn, protocol.MsgExtendedLogin)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(msg.Tests)
}

func (s *httpHandler) SingleServingServer(dir string) (ndt.SingleMeasurementServer, error) {
	return s.serverFactory.SingleServingServer(dir)
}

// ServeHTTP is the command channel for the NDT-WS or NDT-WSS test. All
// subsequent client communication is synchronized with this method. Returning
// closes the websocket connection, so only occurs after all tests complete or
// an unrecoverable error. It is called ServeHTTP to make sure that the Server
// implements the http.Handler interface.
func (s *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upgrader := ws.Upgrader("ndt")
	wsc, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ERROR SERVER:", err)
		return
	}
	ws := protocol.AdaptWsConn(wsc)
	defer warnonerror.Close(ws, "Could not close connection")
	isMon := fmt.Sprintf("%t", controller.IsMonitoring(controller.GetClaim(r.Context())))
	ndt5.HandleControlChannel(ws, s, isMon)
}

// NewWS returns a handler suitable for http-based connections.
func NewWS(datadir string, metadata []metadata.NameValue) WSHandler {
	return &httpHandler{
		serverFactory:  &httpFactory{},
		connectionType: ndt.WS,
		datadir:        datadir,
		metadata:       metadata,
	}
}

type httpsFactory struct {
	certFile string
	keyFile  string
}

func (hf *httpsFactory) SingleServingServer(dir string) (ndt.SingleMeasurementServer, error) {
	return singleserving.ListenWSS(dir, hf.certFile, hf.keyFile)
}

// NewWSS returns a handler suitable for https-based connections.
func NewWSS(datadir, certFile, keyFile string, metadata []metadata.NameValue) WSHandler {
	return &httpHandler{
		serverFactory: &httpsFactory{
			certFile: certFile,
			keyFile:  keyFile,
		},
		connectionType: ndt.WSS,
		datadir:        datadir,
		metadata:       metadata,
	}
}
