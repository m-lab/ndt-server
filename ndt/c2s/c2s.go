package c2s

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-cloud/ndt/metrics"
	"github.com/m-lab/ndt-cloud/ndt/protocol"
	"github.com/m-lab/ndt-cloud/ndt/testresponder"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Responder struct {
	testresponder.TestResponder
}

// TestServer performs the NDT c2s test.
func (tr *Responder) TestServer(w http.ResponseWriter, r *http.Request) {
	upgrader := testresponder.MakeNdtUpgrader([]string{"c2s"})
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade should have already returned an HTTP error code.
		log.Println("ERROR C2S: upgrader", err)
		return
	}
	defer ws.Close()
	tr.Response <- testresponder.Ready
	bytesPerSecond := tr.recvC2SUntil(ws)
	tr.Response <- bytesPerSecond

	// Drain client for a few more seconds, and discard results.
	deadline, _ := tr.Ctx.Deadline()
	tr.Cancel()
	tr.Ctx, tr.Cancel = context.WithDeadline(context.Background(), deadline)
	_ = tr.recvC2SUntil(ws)
}

func (tr *Responder) recvC2SUntil(ws *websocket.Conn) float64 {
	done := make(chan float64)

	go func() {
		totalBytes := float64(0)
		startTime := time.Now()
		endTime := startTime.Add(10 * time.Second)
		for time.Now().Before(endTime) {
			_, buffer, err := ws.ReadMessage()
			if err != nil {
				tr.Cancel()
				return
			}
			totalBytes += float64(len(buffer))
		}
		bytesPerSecond := totalBytes / float64(time.Since(startTime)/time.Second)
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

func ManageTest(ws *websocket.Conn, certFile, keyFile string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Create a testResponder instance.
	testResponder := &Responder{}

	// Create a TLS server for running the C2S test.
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/ndt_protocol",
		promhttp.InstrumentHandlerCounter(
			metrics.TestCount.MustCurryWith(prometheus.Labels{"direction": "c2s"}),
			http.HandlerFunc(testResponder.TestServer)))
	err := testResponder.StartTLSAsync(serveMux, "C2S", certFile, keyFile)
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
