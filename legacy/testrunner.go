package legacy

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-cloud/legacy/c2s"
	"github.com/m-lab/ndt-cloud/legacy/metrics"
	"github.com/m-lab/ndt-cloud/legacy/protocol"
	"github.com/m-lab/ndt-cloud/legacy/s2c"
	"github.com/m-lab/ndt-cloud/legacy/testresponder"
)

const (
	cTestC2S    = 2
	cTestS2C    = 4
	cTestStatus = 16
)

// BasicServer contains everything needed to start a new server on a random port.
type BasicServer struct {
	CertFile string
	KeyFile  string
	TLS      bool
}

// TODO: run meta test.
func runMetaTest(ws *websocket.Conn) {
	var err error
	var message *protocol.JSONMessage

	protocol.SendJSONMessage(protocol.TestPrepare, "", ws)
	protocol.SendJSONMessage(protocol.TestStart, "", ws)
	for {
		message, err = protocol.ReceiveJSONMessage(ws, protocol.TestMsg)
		if message.Msg == "" || err != nil {
			break
		}
		log.Println("Meta message: ", message)
	}
	if err != nil {
		log.Println("Error reading JSON message:", err)
		return
	}
	protocol.SendJSONMessage(protocol.TestFinalize, "", ws)
}

// ServeHTTP is the command channel for the NDT-WS test. All subsequent client
// communication is synchronized with this method. Returning closes the
// websocket connection, so only occurs after all tests complete or an
// unrecoverable error. It is called ServeHTTP to make sure that the Server
// implements the http.Handler interface.
func (s *BasicServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upgrader := testresponder.MakeNdtUpgrader([]string{"ndt"})
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ERROR SERVER:", err)
		return
	}
	defer ws.Close()
	config := &testresponder.Config{
		TLS:      s.TLS,
		CertFile: s.CertFile,
		KeyFile:  s.KeyFile,
	}

	message, err := protocol.ReceiveJSONMessage(ws, protocol.MsgExtendedLogin)
	if err != nil {
		log.Println("Error reading JSON message:", err)
		return
	}
	tests, err := strconv.ParseInt(message.Tests, 10, 64)
	if err != nil {
		log.Println("Failed to parse Tests integer:", err)
		return
	}
	if (tests & cTestStatus) == 0 {
		log.Println("We don't support clients that don't support cTestStatus")
		return
	}
	testsToRun := []string{}
	runC2s := (tests & cTestC2S) != 0
	runS2c := (tests & cTestS2C) != 0

	if runC2s {
		testsToRun = append(testsToRun, strconv.Itoa(cTestC2S))
	}
	if runS2c {
		testsToRun = append(testsToRun, strconv.Itoa(cTestS2C))
	}

	protocol.SendJSONMessage(protocol.SrvQueue, "0", ws)
	protocol.SendJSONMessage(protocol.MsgLogin, "v5.0-NDTinGO", ws)
	protocol.SendJSONMessage(protocol.MsgLogin, strings.Join(testsToRun, " "), ws)

	var c2sRate, s2cRate float64
	if runC2s {
		c2sRate, err = c2s.ManageTest(ws, config)
		if err != nil {
			log.Println("ERROR: manageC2sTest", err)
		} else {
			metrics.TestRate.WithLabelValues("c2s").Observe(c2sRate / 1000.0)
		}
	}
	if runS2c {
		s2cRate, err = s2c.ManageTest(ws, config)
		if err != nil {
			log.Println("ERROR: manageS2cTest", err)
		} else {
			metrics.TestRate.WithLabelValues("s2c").Observe(s2cRate / 1000.0)
		}
	}
	log.Printf("NDT: %s uploaded at %.4f and downloaded at %.4f", r.RemoteAddr, c2sRate, s2cRate)
	protocol.SendJSONMessage(protocol.MsgResults, fmt.Sprintf("You uploaded at %.4f and downloaded at %.4f", c2sRate, s2cRate), ws)
	protocol.SendJSONMessage(protocol.MsgLogout, "", ws)
}
