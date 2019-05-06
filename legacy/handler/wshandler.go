package handler

import (
	"log"
	"net/http"

	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/legacy"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/singleserving"
	"github.com/m-lab/ndt-server/legacy/ws"
)

type httpFactory struct{}

func (hf *httpFactory) SingleServingServer(dir string) (singleserving.Server, error) {
	return singleserving.StartWS(dir)
}

// httpHandler handles requests that come in over HTTP or HTTPS. It should be
// created with MakeHTTPHandler() or MakeHTTPSHandler().
type httpHandler struct {
	serverFactory singleserving.Factory
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
	legacy.HandleControlChannel(ws, s.serverFactory)
}

// NewWS returns a handler suitable for http-based connections.
func NewWS() http.Handler {
	return &httpHandler{
		serverFactory: &httpFactory{},
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
func NewWSS(certFile, keyFile string) http.Handler {
	return &httpHandler{
		serverFactory: &httpsFactory{
			certFile: certFile,
			keyFile:  keyFile,
		},
	}
}
