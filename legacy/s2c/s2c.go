package s2c

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/legacy/ndt"
	"github.com/m-lab/ndt-server/legacy/protocol"
)

// ArchivalData is the data saved by the S2C test. If a researcher wants deeper
// data, then they should use the UUID to get deeper data from tcp-info.
type ArchivalData struct {
	// The server and client IP are here as well as in the containing struct
	// because happy eyeballs means that we may have a IPv4 control connection
	// causing a IPv6 connection to the test port or vice versa.
	ServerIP string
	ClientIP string

	// This is the only field that is really required.
	TestConnectionUUID string

	// These fields are here to enable analyses that don't require joining with tcp-info data.
	StartTime                        time.Time
	EndTime                          time.Time
	MeanThroughputMbps               float64
	MinRTT                           time.Duration
	ClientReportedMeanThroughputMbps float64
	// TODO: Add MaxThroughputKbps and Jitter
	Error string `json:",omitempty"`
}

// result is the result object returned to S2C clients as JSON.
type result struct {
	ThroughputValue  string
	UnsentDataAmount string
	TotalSentByte    string
}

func (n *result) String() string {
	b, _ := json.Marshal(n)
	return string(b)
}

// ManageTest manages the s2c test lifecycle
func ManageTest(ctx context.Context, conn protocol.Connection, s ndt.Server) (*ArchivalData, error) {
	localCtx, localCancel := context.WithTimeout(ctx, 30*time.Second)
	defer localCancel()
	record := &ArchivalData{}

	srv, err := s.SingleServingServer("s2c")
	if err != nil {
		log.Println("Could not start single serving server", err)
		record.Error = err.Error()
		return record, err
	}
	err = protocol.SendJSONMessage(protocol.TestPrepare, strconv.Itoa(srv.Port()), conn)
	if err != nil {
		log.Println("Could not send TestPrepare", err)
		record.Error = err.Error()
		return record, err
	}

	testConn, err := srv.ServeOnce(localCtx)
	if err != nil || testConn == nil {
		log.Println("Could not successfully ServeOnce", err)
		record.Error = err.Error()
		return record, err
	}
	defer warnonerror.Close(testConn, "Could not close test connection")
	record.TestConnectionUUID = conn.UUID()
	record.ServerIP = conn.ServerIP()
	record.ClientIP = conn.ClientIP()

	dataToSend := make([]byte, 8192)
	for i := range dataToSend {
		dataToSend[i] = byte(((i * 101) % (122 - 33)) + 33)
	}

	err = protocol.SendJSONMessage(protocol.TestStart, "", conn)
	if err != nil {
		log.Println("Could not write TestStart", err)
		record.Error = err.Error()
		return record, err
	}

	testConn.StartMeasuring(localCtx)
	record.StartTime = time.Now()
	byteCount, err := testConn.FillUntil(time.Now().Add(10*time.Second), dataToSend)
	record.EndTime = time.Now()
	if err != nil {
		log.Println("Could not FillUntil", err)
		record.Error = err.Error()
		return record, err
	}

	metrics, err := testConn.StopMeasuring()
	if err != nil {
		log.Println("Could not read metrics", err)
		record.Error = err.Error()
		return record, err
	}

	bps := 8 * float64(byteCount) / 10
	kbps := bps / 1000
	record.MinRTT = time.Duration(metrics.MinRTT) * time.Millisecond
	record.MeanThroughputMbps = kbps / 1000 // Convert Kbps to Mbps

	// Send additional download results to the client.
	resultMsg := &result{
		// TODO: clean up this logic to use socket stats rather than application-level counters.
		ThroughputValue:  strconv.FormatInt(int64(kbps), 10),
		UnsentDataAmount: "0",
		TotalSentByte:    strconv.FormatInt(byteCount, 10), // TODO: use actual bytes sent.
	}
	err = protocol.WriteNDTMessage(conn, protocol.TestMsg, resultMsg)
	if err != nil {
		log.Println("Could not write a TestMsg", err)
		record.Error = err.Error()
		return record, err
	}

	clientRateMsg, err := protocol.ReceiveJSONMessage(conn, protocol.TestMsg)
	if err != nil {
		log.Println("Could not receive a TestMsg", err)
		record.Error = err.Error()
		return record, err
	}
	log.Println("We measured", kbps, "and the client sent us", clientRateMsg)
	clientRateKbps, err := strconv.ParseFloat(clientRateMsg.Msg, 64)
	if err == nil {
		record.ClientReportedMeanThroughputMbps = clientRateKbps / 1000
	} else {
		log.Println("Could not parse number sent from client")
		// Being unable to parse the number should not be a fatal error, so continue.
	}

	err = protocol.SendMetrics(metrics, conn)
	if err != nil {
		log.Println("Could not SendMetrics", err)
		record.Error = err.Error()
		return record, err
	}

	err = protocol.SendJSONMessage(protocol.TestFinalize, "", conn)
	if err != nil {
		log.Println("Could not send TestFinalize", err)
		record.Error = err.Error()
		return record, err
	}

	return record, nil
}
