package s2c

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/legacy/ndt"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/metrics"
)

// ArchivalData is the data saved by the S2C test. If a researcher wants deeper
// data, then they should use the UUID to get deeper data from tcp-info.
type ArchivalData struct {
	// This is the only field that is really required.
	TestConnectionUUID string

	// All subsequent fields are here to enable analyses that don't require joining
	// with tcp-info data.

	// The server and client IP are here as well as in the containing struct
	// because happy eyeballs means that we may have a IPv4 control connection
	// causing a IPv6 connection to the test port or vice versa.
	ServerIP string
	ClientIP string

	StartTime          time.Time
	EndTime            time.Time
	MeanThroughputMbps float64
	MinRTT             time.Duration
	ClientReportedMbps float64
	// TODO: Add TCPEngine (bbr, cubic, reno, etc.), MaxThroughputKbps, and Jitter

	Error string `json:",omitempty"`
}

// ManageTest manages the s2c test lifecycle
func ManageTest(ctx context.Context, controlConn protocol.Connection, s ndt.Server) (record *ArchivalData, err error) {
	localCtx, localCancel := context.WithTimeout(ctx, 30*time.Second)
	defer localCancel()
	record = &ArchivalData{}
	defer func() {
		if err != nil {
			record.Error = err.Error()
		}
	}()

	srv, err := s.SingleServingServer("s2c")
	if err != nil {
		log.Println("Could not start single serving server", err)
		metrics.ErrorCount.WithLabelValues("s2c", "StartSingleServingServer").Inc()
		return record, err
	}
	m := controlConn.Messager()
	err = m.SendMessage(protocol.TestPrepare, []byte(strconv.Itoa(srv.Port())))
	if err != nil {
		log.Println("Could not send TestPrepare", err)
		metrics.ErrorCount.WithLabelValues("s2c", "TestPrepare").Inc()
		return record, err
	}

	testConn, err := srv.ServeOnce(localCtx)
	if err != nil || testConn == nil {
		log.Println("Could not successfully ServeOnce", err)
		metrics.ErrorCount.WithLabelValues("s2c", "ServeOnce").Inc()
		if err == nil {
			err = errors.New("nil testConn, but also a nil error")
		}
		return record, err
	}
	defer warnonerror.Close(testConn, "Could not close test connection")
	record.TestConnectionUUID = testConn.UUID()
	record.ServerIP = testConn.ServerIP()
	record.ClientIP = testConn.ClientIP()

	dataToSend := make([]byte, 8192)
	for i := range dataToSend {
		dataToSend[i] = byte(((i * 101) % (122 - 33)) + 33)
	}

	err = m.SendMessage(protocol.TestStart, []byte{})
	if err != nil {
		log.Println("Could not write TestStart", err)
		metrics.ErrorCount.WithLabelValues("s2c", "TestStart").Inc()
		return record, err
	}

	testConn.StartMeasuring(localCtx)
	record.StartTime = time.Now()
	byteCount, err := testConn.FillUntil(time.Now().Add(10*time.Second), dataToSend)
	record.EndTime = time.Now()
	if err != nil {
		log.Println("Could not FillUntil", err)
		metrics.ErrorCount.WithLabelValues("s2c", "FillUntil").Inc()
		return record, err
	}

	web100metrics, err := testConn.StopMeasuring()
	if err != nil {
		log.Println("Could not read metrics", err)
		metrics.ErrorCount.WithLabelValues("s2c", "web100Metrics").Inc()
		return record, err
	}

	bps := 8 * float64(byteCount) / 10
	kbps := bps / 1000
	record.MinRTT = time.Duration(web100metrics.MinRTT) * time.Millisecond
	record.MeanThroughputMbps = kbps / 1000 // Convert Kbps to Mbps

	// Send download results to the client.
	// TODO: clean up this logic to use socket stats rather than application-level
	// counters and the actual bytes sent.
	err = m.SendS2CResults(int64(kbps), 0, byteCount)
	if err != nil {
		log.Println("Could not write a TestMsg", err)
		metrics.ErrorCount.WithLabelValues("s2c", "TestMsgSend").Inc()
		return record, err
	}

	clientRateMsg, err := m.ReceiveMessage(protocol.TestMsg)
	if err != nil {
		metrics.ErrorCount.WithLabelValues("s2c", "TestMsgRcv").Inc()
		log.Println("Could not receive a TestMsg", err)
		return record, err
	}
	log.Println("We measured", kbps, "and the client sent us", clientRateMsg)
	clientRateKbps, err := strconv.ParseFloat(string(clientRateMsg), 64)
	if err == nil {
		record.ClientReportedMbps = clientRateKbps / 1000
	} else {
		log.Println("Could not parse number sent from client")
		// Being unable to parse the number should not be a fatal error, so continue.
	}

	err = protocol.SendMetrics(web100metrics, m)
	if err != nil {
		log.Println("Could not SendMetrics", err)
		metrics.ErrorCount.WithLabelValues("s2c", "SendMetrics").Inc()
		return record, err
	}

	err = m.SendMessage(protocol.TestFinalize, []byte{})
	if err != nil {
		log.Println("Could not send TestFinalize", err)
		metrics.ErrorCount.WithLabelValues("s2c", "TestFinalize").Inc()
		return record, err
	}

	return record, nil
}
