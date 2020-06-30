package s2c

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/ndt5/metrics"
	"github.com/m-lab/ndt-server/ndt5/ndt"
	"github.com/m-lab/ndt-server/ndt5/protocol"
	"github.com/m-lab/tcp-info/tcp"
)

// ArchivalData is the data saved by the S2C test. If a researcher wants deeper
// data, then they should use the UUID to get deeper data from tcp-info.
type ArchivalData struct {
	// This is the only field that is really required.
	UUID string

	// All subsequent fields are here to enable analyses that don't require joining
	// with tcp-info data.

	// The server and client IP are here as well as in the containing struct
	// because happy eyeballs means that we may have a IPv4 control connection
	// causing a IPv6 connection to the test port or vice versa.
	ServerIP   string
	ServerPort int
	ClientIP   string
	ClientPort int

	StartTime          time.Time
	EndTime            time.Time
	MeanThroughputMbps float64
	MinRTT             time.Duration
	MaxRTT             time.Duration
	SumRTT             time.Duration
	CountRTT           uint32
	ClientReportedMbps float64
	// TODO: Add TCPEngine (bbr, cubic, reno, etc.), MaxThroughputKbps, and Jitter

	TCPInfo *tcp.LinuxTCPInfo `json:",omitempty"`
	Error   string            `json:",omitempty"`
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

	connType := s.ConnectionType().Label()

	srv, err := s.SingleServingServer("s2c")
	if err != nil {
		log.Println("Could not start single serving server", err)
		metrics.ClientTestErrors.WithLabelValues(connType, "s2c", "StartSingleServingServer").Inc()
		return record, err
	}
	m := controlConn.Messager()
	err = m.SendMessage(protocol.TestPrepare, []byte(strconv.Itoa(srv.Port())))
	if err != nil {
		log.Println("Could not send TestPrepare", err)
		metrics.ClientTestErrors.WithLabelValues(connType, "s2c", "TestPrepare").Inc()
		return record, err
	}

	testConn, err := srv.ServeOnce(localCtx)
	if err != nil || testConn == nil {
		log.Println("Could not successfully ServeOnce", err)
		metrics.ClientTestErrors.WithLabelValues(connType, "s2c", "ServeOnce").Inc()
		if err == nil {
			err = errors.New("nil testConn, but also a nil error")
		}
		return record, err
	}
	record.UUID = testConn.UUID()
	record.ServerIP, record.ServerPort = testConn.ServerIPAndPort()
	record.ClientIP, record.ClientPort = testConn.ClientIPAndPort()

	dataToSend := make([]byte, 8192)
	for i := range dataToSend {
		dataToSend[i] = byte(((i * 101) % (122 - 33)) + 33)
	}

	err = m.SendMessage(protocol.TestStart, []byte{})
	if err != nil {
		warnonerror.Close(testConn, "Could not close test connection")
		log.Println("Could not write TestStart", err, record.UUID)
		metrics.ClientTestErrors.WithLabelValues(connType, "s2c", "TestStart").Inc()
		return record, err
	}

	testConn.StartMeasuring(localCtx)
	record.StartTime = time.Now()
	testConn.FillUntil(time.Now().Add(10*time.Second), dataToSend)
	record.EndTime = time.Now()

	web100metrics, err := testConn.StopMeasuring()
	if err != nil {
		warnonerror.Close(testConn, "Could not close test connection")
		log.Println("Could not read metrics", err, record.UUID)
		metrics.ClientTestErrors.WithLabelValues(connType, "s2c", "web100Metrics").Inc()
		return record, err
	}

	// Close the test connection to signal to single-threaded clients that the
	// download has completed. Note: a possible optimization is to wait for
	// one-two seconds for the client to close the connection and then close
	// it anyway. This gives us the advantage that the client will retain
	// the state assciated with initiating the close.
	warnonerror.Close(testConn, "Could not close testConnection")

	// Bits per second is the number of bits divided by the duration of the
	// test.  The duration of the test is supposed to be 10 seconds, but it
	// can vary in practice, so we divide by the actual duration instead of
	// assuming it was 10.
	bps := 8 * float64(web100metrics.TCPInfo.BytesAcked) / record.EndTime.Sub(record.StartTime).Seconds()
	kbps := bps / 1000
	record.MinRTT = time.Duration(web100metrics.MinRTT) * time.Millisecond
	record.MaxRTT = time.Duration(web100metrics.MaxRTT) * time.Millisecond
	record.SumRTT = time.Duration(web100metrics.SumRTT) * time.Millisecond
	record.CountRTT = web100metrics.CountRTT
	record.MeanThroughputMbps = kbps / 1000 // Convert Kbps to Mbps
	record.TCPInfo = &web100metrics.TCPInfo

	// Send download results to the client.
	err = m.SendS2CResults(int64(kbps), 0, web100metrics.TCPInfo.BytesAcked)
	if err != nil {
		log.Println("Could not write a TestMsg", err, record.UUID)
		metrics.ClientTestErrors.WithLabelValues(connType, "s2c", "TestMsgSend").Inc()
		return record, err
	}

	clientRateMsg, err := m.ReceiveMessage(protocol.TestMsg)
	// Do not return with an error if we got anything at all from the client.
	if err != nil && clientRateMsg == nil {
		metrics.ClientTestErrors.WithLabelValues(connType, "s2c", "TestMsgRcv").Inc()
		log.Println("Could not receive a TestMsg", err, record.UUID)
		return record, err
	}
	log.Println("We measured", kbps, "and the client sent us", string(clientRateMsg))
	clientRateKbps, err := strconv.ParseFloat(string(clientRateMsg), 64)
	if err == nil {
		record.ClientReportedMbps = clientRateKbps / 1000
	} else {
		log.Println("Could not parse number sent from client")
		// Being unable to parse the number should not be a fatal error, so continue.
	}

	err = protocol.SendMetrics(web100metrics, m, "")
	if err != nil {
		log.Println("Could not SendMetrics for the legacy data", err, record.UUID)
		metrics.ClientTestErrors.WithLabelValues(connType, "s2c", "SendMetricsLegacy").Inc()
		return record, err
	}
	err = protocol.SendMetrics(record, m, "NDTResult.S2C.")
	if err != nil {
		log.Println("Could not SendMetrics for the archival data", err, record.UUID)
		metrics.ClientTestErrors.WithLabelValues(connType, "s2c", "SendMetricsArchival").Inc()
		return record, err
	}

	err = m.SendMessage(protocol.TestFinalize, []byte{})
	if err != nil {
		log.Println("Could not send TestFinalize", err, record.UUID)
		metrics.ClientTestErrors.WithLabelValues(connType, "s2c", "TestFinalize").Inc()
		return record, err
	}

	return record, nil
}
