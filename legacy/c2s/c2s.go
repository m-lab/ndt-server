package c2s

import (
	"context"
	"fmt"
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

// Responder responds to c2s tests.
type Responder struct {
	testresponder.TestResponder
}

// TestServer performs the NDT c2s test.
func (tr *Responder) TestServer(w http.ResponseWriter, r *http.Request) {
	upgrader := testresponder.MakeNdtUpgrader([]string{"c2s"})
	wsc, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade should have already returned an HTTP error code.
		log.Println("ERROR C2S: upgrader", err)
		return
	}
	ws := protocol.AdaptWsConn(wsc)
	tr.PerformTest(ws)
	ws.Close()
}

func (tr *Responder) PerformTest(ws protocol.Connection) {
	tr.Response <- testresponder.Ready
	bytesPerSecond := tr.recvC2SUntil(ws)
	tr.Response <- bytesPerSecond

	// Drain client for a few more seconds, and discard results.
	deadline, _ := tr.Ctx.Deadline()
	tr.Cancel()
	tr.Ctx, tr.Cancel = context.WithDeadline(context.Background(), deadline)
	_ = tr.recvC2SUntil(ws)
}

func (tr *Responder) recvC2SUntil(ws protocol.Connection) float64 {
	done := make(chan float64)

	go func() {
		startTime := time.Now()
		endTime := startTime.Add(10 * time.Second)
		totalBytes, err := ws.DrainUntil(endTime)
		if err != nil {
			tr.Cancel()
			return
		}
		bytesPerSecond := float64(totalBytes) / float64(time.Since(startTime)/time.Second)
		done <- bytesPerSecond
	}()

	log.Println("C2S: Waiting for test to complete or timeout")
	select {
	case <-tr.Ctx.Done():
		log.Println("C2S: Context Done!", tr.Ctx.Err())
		ws.Close()
		// Return zero on error.
		return 0
	case bytesPerSecond := <-done:
		return bytesPerSecond
	}
}

// ManageTest manages the c2s test lifecycle.
func ManageTest(ws protocol.Connection, config *testresponder.Config) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Create a testResponder instance.
	testResponder := &Responder{}
	testResponder.Config = config

	// Create a TLS server for running the C2S test.
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/ndt_protocol",
		promhttp.InstrumentHandlerCounter(
			metrics.TestCount.MustCurryWith(prometheus.Labels{"direction": "c2s"}),
			http.HandlerFunc(testResponder.TestServer)))
	err := testResponder.StartAsync(serveMux, testResponder.PerformTest, "C2S")
	if err != nil {
		return 0, err
	}
	defer testResponder.Close()

	done := make(chan float64)
	go func() {
		// Wait for test to run. ///////////////////////////////////////////
		// Send the server port to the client.
		protocol.SendJSONMessage(protocol.TestPrepare, strconv.Itoa(testResponder.Port), ws)
		c2sReady := <-testResponder.Response
		if c2sReady != testresponder.Ready {
			log.Println("ERROR C2S: Bad value received on the c2s channel", c2sReady)
			cancel()
			return
		}
		protocol.SendJSONMessage(protocol.TestStart, "", ws)
		c2sBytesPerSecond := <-testResponder.Response
		c2sKbps := 8 * c2sBytesPerSecond / 1000.0

		protocol.SendJSONMessage(protocol.TestMsg, fmt.Sprintf("%.4f", c2sKbps), ws)
		protocol.SendJSONMessage(protocol.TestFinalize, "", ws)
		log.Println("C2S: server rate:", c2sKbps)
		done <- c2sKbps
	}()

	select {
	case <-ctx.Done():
		log.Println("C2S: ctx Done!")
		return 0, ctx.Err()
	case value := <-done:
		log.Println("C2S: finished ", value)
		return value, nil
	}
}
