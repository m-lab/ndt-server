package legacy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/ndt-cloud/ndt"
)

// C2STestHandler is an http.Handler that executes the NDT c2s test over websockets.
func (tr *Responder) C2STestHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := MakeNdtUpgrader([]string{"c2s"})
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade should have already returned an HTTP error code.
		log.Println("ERROR C2S: upgrader", err)
		return
	}
	defer ws.Close()

	// Define an absolute deadline for running all tests.
	deadline := time.Now().Add(tr.duration)

	// Signal ready, and run the test.
	tr.result <- cReadyC2S
	bytesPerSecond := runC2S(ws, deadline.Sub(time.Now()), true)
	tr.result <- bytesPerSecond

	// Drain client for a few more seconds, and discard results.
	_ = runC2S(ws, deadline.Sub(time.Now()), false)
}

// C2SController manages communication with the C2STestHandler from the control
// channel.
func (tr *Responder) C2SController(ws *websocket.Conn) (float64, error) {
	// Wait for test to run.
	// Send the server port to the client.
	SendNdtMessage(ndt.TestPrepare, strconv.Itoa(tr.port), ws)
	c2sReady := <-tr.result
	if c2sReady != cReadyC2S {
		return 0, fmt.Errorf("ERROR C2S: Bad value received on the c2s channel: %f", c2sReady)
	}
	SendNdtMessage(ndt.TestStart, "", ws)
	c2sBytesPerSecond := <-tr.result
	c2sKbps := 8 * c2sBytesPerSecond / 1000.0

	SendNdtMessage(ndt.TestMsg, fmt.Sprintf("%.4f", c2sKbps), ws)
	SendNdtMessage(ndt.TestFinalize, "", ws)
	log.Println("C2S: server rate:", c2sKbps)
	return c2sKbps, nil
}

// runC2S performs a 10 second NDT client to server test. Runtime is
// guaranteed to be no more than timeout. The timeout should be slightly greater
// than 10 sec. The given websocket should be closed by the caller.
func runC2S(ws *websocket.Conn, timeout time.Duration, logErrors bool) float64 {
	done := make(chan float64)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Run recv in background.
	go func() {
		bytesPerSec, err := recvUntil(ws, 10*time.Second)
		if err != nil {
			cancel()
			if logErrors {
				log.Println("C2S: recvUntil error:", err)
			}
			return
		}
		done <- bytesPerSec
	}()

	select {
	case <-ctx.Done():
		if logErrors {
			log.Println("C2S: Context Done!", ctx.Err())
		}
		// Return zero on error.
		return 0
	case bytesPerSecond := <-done:
		return bytesPerSecond
	}
}

// recvUntil reads from the given websocket for duration seconds and returns the
// average rate.
func recvUntil(ws *websocket.Conn, duration time.Duration) (float64, error) {
	totalBytes := float64(0)
	startTime := time.Now()
	endTime := startTime.Add(duration)
	for time.Now().Before(endTime) {
		_, buffer, err := ws.ReadMessage()
		if err != nil {
			return 0, err
		}
		totalBytes += float64(len(buffer))
	}
	bytesPerSecond := totalBytes / float64(time.Since(startTime)/time.Second)
	return bytesPerSecond, nil
}
