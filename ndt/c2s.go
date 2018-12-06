package ndt

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-cloud/ndt/protocol"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// C2STestServer performs the NDT c2s test.
func (tr *TestResponder) C2STestServer(w http.ResponseWriter, r *http.Request) {
	upgrader := makeNdtUpgrader([]string{"c2s"})
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade should have already returned an HTTP error code.
		log.Println("ERROR C2S: upgrader", err)
		return
	}
	defer ws.Close()
	tr.response <- cReadyC2S
	bytesPerSecond := tr.recvC2SUntil(ws)
	tr.response <- bytesPerSecond

	// Drain client for a few more seconds, and discard results.
	deadline, _ := tr.ctx.Deadline()
	tr.cancel()
	tr.ctx, tr.cancel = context.WithDeadline(context.Background(), deadline)
	_ = tr.recvC2SUntil(ws)
}

func (tr *TestResponder) recvC2SUntil(ws *websocket.Conn) float64 {
	done := make(chan float64)

	go func() {
		totalBytes := float64(0)
		startTime := time.Now()
		endTime := startTime.Add(10 * time.Second)
		for time.Now().Before(endTime) {
			_, buffer, err := ws.ReadMessage()
			if err != nil {
				tr.cancel()
				return
			}
			totalBytes += float64(len(buffer))
		}
		bytesPerSecond := totalBytes / float64(time.Since(startTime)/time.Second)
		done <- bytesPerSecond
	}()

	log.Println("C2S: Waiting for test to complete or timeout")
	select {
	case <-tr.ctx.Done():
		log.Println("C2S: Context Done!", tr.ctx.Err())
		ws.Close()
		// Return zero on error.
		return 0
	case bytesPerSecond := <-done:
		return bytesPerSecond
	}
}

func (s *Server) manageC2sTest(ws *websocket.Conn) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Create a testResponder instance.
	testResponder := &TestResponder{}

	// Create a TLS server for running the C2S test.
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/ndt_protocol",
		promhttp.InstrumentHandlerCounter(
			testCount.MustCurryWith(prometheus.Labels{"direction": "c2s"}),
			http.HandlerFunc(testResponder.C2STestServer)))
	err := testResponder.StartTLSAsync(serveMux, "C2S", s.CertFile, s.KeyFile)
	if err != nil {
		return 0, err
	}
	defer testResponder.Close()

	done := make(chan float64)
	go func() {
		// Wait for test to run. ///////////////////////////////////////////
		// Send the server port to the client.
		protocol.SendJSONMessage(protocol.TestPrepare, strconv.Itoa(testResponder.Port()), ws)
		c2sReady := <-testResponder.response
		if c2sReady != cReadyC2S {
			log.Println("ERROR C2S: Bad value received on the c2s channel", c2sReady)
			cancel()
			return
		}
		protocol.SendJSONMessage(protocol.TestStart, "", ws)
		c2sBytesPerSecond := <-testResponder.response
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
