package s2c

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/m-lab/ndt-server/legacy/metrics"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/testresponder"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Responder is a response for the s2c test
type Responder struct {
	testresponder.TestResponder
}

// Result is the result object returned to S2C clients as JSON.
type Result struct {
	ThroughputValue  string
	UnsentDataAmount string
	TotalSentByte    string
}

func (n *Result) String() string {
	b, _ := json.Marshal(n)
	return string(b)
}

// websocketHandler performs the NDT s2c test.
func (s2c *Responder) websocketHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := testresponder.MakeNdtUpgrader([]string{"s2c"})
	wsc, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade should have already returned an HTTP error code.
		log.Println("ERROR S2C: upgrader", err)
		return
	}
	ws := protocol.AdaptWsConn(wsc)
	s2c.PerformTest(ws)
}

func (s2c *Responder) PerformTest(ws protocol.Connection) {
	dataToSend := make([]byte, 81920)
	for i := range dataToSend {
		dataToSend[i] = byte(((i * 101) % (122 - 33)) + 33)
	}

	// Signal control channel that we are about to start the test.
	s2c.Response <- testresponder.Ready

	// Create ticker to enforce timeout on
	done := make(chan float64)

	go func() {
		startTime := time.Now()
		endTime := startTime.Add(10 * time.Second)
		totalBytes, err := ws.FillUntil(endTime, dataToSend)
		if err != nil {
			log.Println("ERROR S2C: sending message", err)
			s2c.Cancel()
			return
		}
		done <- float64(totalBytes) / float64(time.Since(startTime)/time.Second)
	}()

	log.Println("S2C: Waiting for test to complete or timeout")
	select {
	case <-s2c.Ctx.Done():
		log.Println("S2C: Context Done!", s2c.Ctx.Err())
		// Return zero on error.
		s2c.Response <- 0
	case bytesPerSecond := <-done:
		log.Println("S2C: Ran test and measured a download speed of", bytesPerSecond)
		s2c.Response <- bytesPerSecond
	}
	ws.Close()
}

// ManageTest manages the s2c test lifecycle
func ManageTest(ws protocol.Connection, config *testresponder.Config) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a testResponder instance.
	testResponder := &Responder{}
	testResponder.Config = config

	// Create a TLS server for running the S2C test.
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/ndt_protocol",
		promhttp.InstrumentHandlerCounter(
			metrics.TestCount.MustCurryWith(prometheus.Labels{"direction": "s2c"}),
			http.HandlerFunc(testResponder.websocketHandler)))
	err := testResponder.StartAsync(serveMux, testResponder.PerformTest, "S2C")
	if err != nil {
		return 0, err
	}
	defer testResponder.Close()

	done := make(chan float64)
	go func() {
		// Wait for test to run. ///////////////////////////////////////////
		// Send the server port to the client.
		protocol.SendJSONMessage(protocol.TestPrepare, strconv.Itoa(testResponder.Port), ws)
		s2cReady := <-testResponder.Response
		if s2cReady != testresponder.Ready {
			log.Println("ERROR S2C: Bad value received on the s2c channel", s2cReady)
			cancel()
			return
		}
		protocol.SendJSONMessage(protocol.TestStart, "", ws)
		s2cBytesPerSecond := <-testResponder.Response
		s2cKbps := 8 * s2cBytesPerSecond / 1000.0

		// Send additional download results to the client.
		resultMsg := &Result{
			ThroughputValue:  strconv.FormatInt(int64(s2cKbps), 10),
			UnsentDataAmount: "0",
			TotalSentByte:    strconv.FormatInt(int64(10*s2cBytesPerSecond), 10), // TODO: use actual bytes sent.
		}
		err = protocol.WriteNDTMessage(ws, protocol.TestMsg, resultMsg)
		if err != nil {
			log.Println("S2C: Failed to write JSON message:", err)
			cancel()
			return
		}
		clientRateMsg, err := protocol.ReceiveJSONMessage(ws, protocol.TestMsg)
		if err != nil {
			log.Println("S2C: Failed to read JSON message:", err)
			cancel()
			return
		}
		log.Println("S2C: The client sent us:", clientRateMsg.Msg)
		requiredWeb100Vars := []string{"MaxRTT", "MinRTT"}

		for _, web100Var := range requiredWeb100Vars {
			protocol.SendJSONMessage(protocol.TestMsg, web100Var+": 0", ws)
		}
		protocol.SendJSONMessage(protocol.TestFinalize, "", ws)
		clientRate, err := strconv.ParseFloat(clientRateMsg.Msg, 64)
		if err != nil {
			log.Println("S2C: Bad client rate:", err)
			cancel()
			return
		}
		log.Println("S2C: server rate:", s2cKbps, "vs client rate:", clientRate)
		done <- s2cKbps
	}()

	select {
	case <-ctx.Done():
		log.Println("S2C: ctx done!")
		return 0, ctx.Err()
	case rate := <-done:
		log.Println("S2C: finished ", rate)
		return rate, nil
	}
}
