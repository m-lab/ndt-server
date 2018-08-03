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

// S2CTestHandler is an http.Handler that executes the NDT s2c test over websockets.
func (tr *Responder) S2CTestHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := MakeNdtUpgrader([]string{"s2c"})
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade should have already returned an HTTP error code.
		log.Println("ERROR S2C: upgrader", err)
		return
	}
	defer ws.Close()

	dataToSend := make([]byte, 81920)
	for i := range dataToSend {
		dataToSend[i] = byte(((i * 101) % (122 - 33)) + 33)
	}

	// Define an absolute deadline for running all tests.
	deadline := time.Now().Add(tr.duration)

	// Signal control channel that we are about to start the test.
	tr.Result <- cReadyS2C
	tr.Result <- runS2C(ws, dataToSend, deadline.Sub(time.Now()))
}

// S2CController manages communication with the S2CTestHandler from the control
// channel.
func (tr *Responder) S2CController(ws *websocket.Conn) (float64, error) {
	// Wait for test to run. ///////////////////////////////////////////
	// Send the server port to the client.
	SendNdtMessage(ndt.TestPrepare, strconv.Itoa(tr.Port), ws)
	s2cReady := <-tr.Result
	if s2cReady != ndt.ReadyS2C {
		return 0, fmt.Errorf("ERROR S2C: Bad value received on the s2c channel: %f", s2cReady)
	}
	SendNdtMessage(ndt.TestStart, "", ws)
	s2cBytesPerSecond := <-tr.Result
	s2cKbps := 8 * s2cBytesPerSecond / 1000.0

	// Send additional download results to the client.
	resultMsg := &NdtS2CResult{
		ThroughputValue:  s2cKbps,
		UnsentDataAmount: 0,
		TotalSentByte:    int64(10 * s2cBytesPerSecond), // TODO: use actual bytes sent.
	}
	err := WriteNdtMessage(ws, ndt.TestMsg, resultMsg)
	if err != nil {
		return 0, fmt.Errorf("S2C: Failed to write JSON message: %s", err)
	}
	clientRateMsg, err := RecvNdtJSONMessage(ws, ndt.TestMsg)
	if err != nil {
		return 0, fmt.Errorf("S2C: Failed to read JSON message: %s", err)
	}
	log.Println("S2C: The client sent us:", clientRateMsg.Msg)
	requiredWeb100Vars := []string{"MaxRTT", "MinRTT"}

	for _, web100Var := range requiredWeb100Vars {
		SendNdtMessage(ndt.TestMsg, web100Var+": 0", ws)
	}
	SendNdtMessage(ndt.TestFinalize, "", ws)
	clientRate, err := strconv.ParseFloat(clientRateMsg.Msg, 64)
	if err != nil {
		return 0, fmt.Errorf("S2C: Bad client rate: %s", err)
	}
	log.Println("S2C: server rate:", s2cKbps, "vs client rate:", clientRate)
	return s2cKbps, nil
}

// runS2C performs a 10 second NDT server to client test. Runtime is
// guaranteed to be no more than timeout. The timeout should be slightly greater
// than 10 sec. The given websocket should be closed by the caller.
func runS2C(ws *websocket.Conn, data []byte, timeout time.Duration) float64 {
	done := make(chan float64)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	go func() {
		bytesPerSec, err := sendUntil(ws, data, 10*time.Second)
		if err != nil {
			cancel()
			log.Println("S2C: sendUntil error:", err)
			return
		}
		done <- bytesPerSec
	}()

	log.Println("S2C: Waiting for test to complete or timeout")
	select {
	case <-ctx.Done():
		log.Println("S2C: Context Done!", ctx.Err())
		// Return zero on error.
		return 0
	case bytesPerSecond := <-done:
		return bytesPerSecond
	}
}

func sendUntil(ws *websocket.Conn, data []byte, duration time.Duration) (float64, error) {
	msg, err := websocket.NewPreparedMessage(websocket.BinaryMessage, data)
	if err != nil {
		return 0, fmt.Errorf("ERROR S2C: Could not make prepared message: %s", err)
	}

	totalBytes := float64(0)
	startTime := time.Now()
	endTime := startTime.Add(duration)
	for time.Now().Before(endTime) {
		err := ws.WritePreparedMessage(msg)
		if err != nil {
			return 0, fmt.Errorf("ERROR S2C: sending message: %s", err)
		}
		totalBytes += float64(len(data))
	}
	return totalBytes / float64(time.Since(startTime)/time.Second), nil
}
