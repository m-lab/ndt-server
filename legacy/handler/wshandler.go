package handler

import (
	"log"
	"net/http"
	"time"

	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/legacy"
	"github.com/m-lab/ndt-server/legacy/ndt"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/singleserving"
	"github.com/m-lab/ndt-server/legacy/ws"
)

// WSHandler is both an ndt.Server and an http.Handler to allow websocket-based
// NDT tests to be run by Go's http libraries.
type WSHandler interface {
	ndt.Server
	http.Handler
}

type httpFactory struct{}

func (hf *httpFactory) SingleServingServer(dir string) (singleserving.Server, error) {
	return singleserving.StartWS(dir)
}

// httpHandler handles requests that come in over HTTP or HTTPS. It should be
// created with MakeHTTPHandler() or MakeHTTPSHandler().
type httpHandler struct {
	serverFactory  singleserving.Factory
	connectionType ndt.ConnectionType
	datadir        string
}

func (s *httpHandler) DataDir() string                    { return s.datadir }
func (s *httpHandler) ConnectionType() ndt.ConnectionType { return s.connectionType }

func (s *httpHandler) TestLength() time.Duration  { return 10 * time.Second }
func (s *httpHandler) TestMaxTime() time.Duration { return 30 * time.Second }

func (s *httpHandler) SingleServingServer(dir string) (singleserving.Server, error) {
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
	legacy.HandleControlChannel(ws, s)
}

// NewWS returns a handler suitable for http-based connections.
func NewWS(datadir string) WSHandler {
	return &httpHandler{
		serverFactory:  &httpFactory{},
		connectionType: ndt.WS,
		datadir:        datadir,
	}
}

type httpsFactory struct {
	certFile string
	keyFile  string
}

func (hf *httpsFactory) SingleServingServer(dir string) (singleserving.Server, error) {
	return singleserving.StartWSS(dir, hf.certFile, hf.keyFile)
}

// NewWSS returns a handler suitable for https-based connections.
func NewWSS(datadir, certFile, keyFile string) WSHandler {
	return &httpHandler{
		serverFactory: &httpsFactory{
			certFile: certFile,
			keyFile:  keyFile,
		},
		connectionType: ndt.WSS,
		datadir:        datadir,
	}
}
