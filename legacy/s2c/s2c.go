package s2c

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/singleserving"
)

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

// ManageTest manages the s2c test lifecycle
func ManageTest(ctx context.Context, conn protocol.Connection, sf singleserving.Factory) (float64, error) {
	localCtx, localCancel := context.WithTimeout(ctx, 30*time.Second)
	defer localCancel()

	srv, err := sf.SingleServingServer("s2c")
	if err != nil {
		log.Println("Could not start single serving server", err)
		return 0, err
	}
	err = protocol.SendJSONMessage(protocol.TestPrepare, strconv.Itoa(srv.Port()), conn)
	if err != nil {
		log.Println("Could not send TestPrepare", err)
		return 0, err
	}

	testConn, err := srv.ServeOnce(localCtx)
	if err != nil {
		log.Println("Could not successfully ServeOnce", err)
		return 0, err
	}
	defer warnonerror.Close(testConn, "Could not close test connection")

	dataToSend := make([]byte, 8192)
	for i := range dataToSend {
		dataToSend[i] = byte(((i * 101) % (122 - 33)) + 33)
	}

	err = protocol.SendJSONMessage(protocol.TestStart, "", conn)
	if err != nil {
		log.Println("Could not write TestStart", err)
		return 0, err
	}

	testConn.StartMeasuring(localCtx)
	byteCount, err := testConn.FillUntil(time.Now().Add(10*time.Second), dataToSend)
	metrics, metricErr := testConn.StopMeasuring()
	if err != nil {
		log.Println("Could not FillUntil", err)
		return 0, err
	}
	if metricErr != nil {
		log.Println("Could not read metrics", metricErr)
		return 0, metricErr
	}

	bps := 8 * float64(byteCount) / 10
	kbps := bps / 1000

	// Send additional download results to the client.
	resultMsg := &Result{
		ThroughputValue:  strconv.FormatInt(int64(kbps), 10),
		UnsentDataAmount: "0",
		TotalSentByte:    strconv.FormatInt(byteCount, 10), // TODO: use actual bytes sent.
	}
	err = protocol.WriteNDTMessage(conn, protocol.TestMsg, resultMsg)
	if err != nil {
		log.Println("Could not write a TestMsg", err)
		return kbps, err
	}
	clientRateMsg, err := protocol.ReceiveJSONMessage(conn, protocol.TestMsg)
	if err != nil {
		log.Println("Could not receive a TestMsg", err)
		return kbps, err
	}
	log.Println("We measured", kbps, "and the client sent us", clientRateMsg)
	err = protocol.SendMetrics(metrics, conn)
	if err != nil {
		log.Println("Could not SendMetrics", err)
		return kbps, err
	}
	err = protocol.SendJSONMessage(protocol.TestFinalize, "", conn)
	if err != nil {
		log.Println("Could not send TestFinalize", err)
		return kbps, err
	}

	return kbps, nil
}
