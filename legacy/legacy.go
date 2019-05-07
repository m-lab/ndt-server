package legacy

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/m-lab/ndt-server/legacy/c2s"
	legacymetrics "github.com/m-lab/ndt-server/legacy/metrics"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/s2c"
	"github.com/m-lab/ndt-server/legacy/singleserving"
)

const (
	cTestC2S    = 2
	cTestS2C    = 4
	cTestStatus = 16
)

// TODO: run meta test.
func runMetaTest(ws protocol.Connection) {
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

// HandleControlChannel is the "business logic" of an NDT test. It is designed
// to run every test, and to never need to know whether the underlying
// connection is just a TCP socket, a WS connection, or a WSS connection. It
// only needs a connection, and a factory for making single-use servers for
// connections of that same type.
func HandleControlChannel(conn protocol.Connection, sf singleserving.Factory) {
	// Nothing should take more than 45 seconds, and exiting this method should
	// cause all resources used by the test to be reclaimed.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	message, err := protocol.ReceiveJSONMessage(conn, protocol.MsgExtendedLogin)
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
		log.Println("We don't support clients that don't support TestStatus")
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

	protocol.SendJSONMessage(protocol.SrvQueue, "0", conn)
	protocol.SendJSONMessage(protocol.MsgLogin, "v5.0-NDTinGO", conn)
	protocol.SendJSONMessage(protocol.MsgLogin, strings.Join(testsToRun, " "), conn)

	var c2sRate, s2cRate float64
	if runC2s {
		c2sRate, err = c2s.ManageTest(ctx, conn, sf)
		if err != nil {
			log.Println("ERROR: manageC2sTest", err)
		} else {
			legacymetrics.TestRate.WithLabelValues("c2s").Observe(c2sRate / 1000.0)
		}
	}
	if runS2c {
		s2cRate, err = s2c.ManageTest(ctx, conn, sf)
		if err != nil {
			log.Println("ERROR: manageS2cTest", err)
		} else {
			legacymetrics.TestRate.WithLabelValues("s2c").Observe(s2cRate / 1000.0)
		}
	}
	log.Printf("NDT: uploaded at %.4f and downloaded at %.4f", c2sRate, s2cRate)
	protocol.SendJSONMessage(protocol.MsgResults, fmt.Sprintf("You uploaded at %.4f and downloaded at %.4f", c2sRate, s2cRate), conn)
	protocol.SendJSONMessage(protocol.MsgLogout, "", conn)

}
