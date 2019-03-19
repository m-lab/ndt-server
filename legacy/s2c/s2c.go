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
	"github.com/m-lab/ndt-server/legacy/web100"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ready should be a constant, but const initializers may not be nil in Go.
var ready *web100.Metrics

// Responder is a response for the s2c test
type Responder struct {
	testresponder.TestResponder
	Response chan *web100.Metrics
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
func (r *Responder) websocketHandler(w http.ResponseWriter, req *http.Request) {
	upgrader := testresponder.MakeNdtUpgrader([]string{"s2c"})
	wsc, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		// Upgrade should have already returned an HTTP error code.
		log.Println("ERROR S2C: upgrader", err)
		return
	}
	ws := protocol.AdaptWsConn(wsc)
	r.performTest(ws)
}

func (r *Responder) performTest(ws protocol.MeasuredConnection) {
	dataToSend := make([]byte, 8192)
	for i := range dataToSend {
		dataToSend[i] = byte(((i * 101) % (122 - 33)) + 33)
	}

	// Signal control channel that we are about to start the test.
	r.Response <- ready

	// Create ticker to enforce timeout on
	done := make(chan *web100.Metrics)

	go func(r *Responder, ws protocol.MeasuredConnection) {
		ws.StartMeasuring(r.Ctx)
		startTime := time.Now()
		endTime := startTime.Add(10 * time.Second)
		totalBytes, err := ws.FillUntil(endTime, dataToSend)
		if err != nil {
			log.Println("ERROR S2C: sending message", err)
			r.Cancel()
			return
		}
		info, err := ws.StopMeasuring()
		if err != nil {
			r.Cancel()
			return
		}
		info.BytesPerSecond = float64(totalBytes) / float64(time.Since(startTime)/time.Second)
		done <- info
	}(r, ws)

	log.Println("S2C: Waiting for test to complete or timeout")
	select {
	case <-r.Ctx.Done():
		log.Println("S2C: Context Done!", r.Ctx.Err())
		// Return zero on error.
		r.Response <- nil
	case info := <-done:
		log.Println("S2C: Ran test and measured a download speed of", info.BytesPerSecond)
		r.Response <- info
	}
	<-r.Response // Wait to close until we are told to close.
	ws.Close()
}

func (r *Responder) runControlChannel(ctx context.Context, cancel context.CancelFunc, done chan float64, ws protocol.Connection) {
	// Wait for test to run. ///////////////////////////////////////////
	// Send the server port to the client.
	protocol.SendJSONMessage(protocol.TestPrepare, strconv.Itoa(r.Port), ws)
	// Wait for the client to connect to the test port
	s2cReady := <-r.Response
	if s2cReady != ready {
		log.Println("ERROR S2C: Bad value received on the s2c channel", s2cReady)
		cancel()
		return
	}
	protocol.SendJSONMessage(protocol.TestStart, "", ws)
	// Wait for the 10 seconds download test to be complete.
	info := <-r.Response
	if info == nil {
		cancel()
		return
	}
	s2cBytesPerSecond := info.BytesPerSecond
	s2cKbps := 8 * s2cBytesPerSecond / 1000.0

	// Send additional download results to the client.
	resultMsg := &Result{
		ThroughputValue:  strconv.FormatInt(int64(s2cKbps), 10),
		UnsentDataAmount: "0",
		TotalSentByte:    strconv.FormatInt(int64(10*s2cBytesPerSecond), 10), // TODO: use actual bytes sent.
	}
	err := protocol.WriteNDTMessage(ws, protocol.TestMsg, resultMsg)
	if err != nil {
		log.Println("S2C: Failed to write JSON message:", err)
		cancel()
		return
	}
	// Receive the client results.
	clientRateMsg, err := protocol.ReceiveJSONMessage(ws, protocol.TestMsg)
	if err != nil {
		log.Println("S2C: Failed to read JSON message:", err)
		cancel()
		return
	}
	// Send the web100vars (TODO: make these TCP_INFO vars)
	log.Println("S2C: The client sent us:", clientRateMsg.Msg)
	protocol.SendMetrics(info, ws)
	protocol.SendJSONMessage(protocol.TestFinalize, "", ws)
	close(r.Response)
	clientRate, err := strconv.ParseFloat(clientRateMsg.Msg, 64)
	if err != nil {
		log.Println("S2C: Bad client rate:", err)
		cancel()
		return
	}
	log.Println("S2C: server rate:", s2cKbps, "vs client rate:", clientRate)
	done <- s2cKbps

}

// ManageTest manages the s2c test lifecycle
func ManageTest(ws protocol.Connection, config *testresponder.Config) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a testResponder instance.
	responder := &Responder{
		Response: make(chan *web100.Metrics),
	}
	responder.Config = config

	// Create a TLS server for running the S2C test.
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/ndt_protocol",
		promhttp.InstrumentHandlerCounter(
			metrics.TestCount.MustCurryWith(prometheus.Labels{"direction": "s2c"}),
			http.HandlerFunc(responder.websocketHandler)))
	err := responder.StartAsync(serveMux, responder.performTest, "S2C")
	if err != nil {
		return 0, err
	}
	defer responder.Close()

	done := make(chan float64)
	go responder.runControlChannel(ctx, cancel, done, ws)

	select {
	case <-ctx.Done():
		log.Println("S2C: ctx done!")
		return 0, ctx.Err()
	case rate := <-done:
		log.Println("S2C: finished ", rate)
		return rate, nil
	}
}
