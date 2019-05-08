package c2s

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/legacy/ndt"
	"github.com/m-lab/ndt-server/legacy/protocol"
)

// ArchivalData is the data saved by the C2S test. If a researcher wants deeper
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
	StartTime          time.Time
	EndTime            time.Time
	MeanThroughputMbps float64
	Error              string `json:",omitempty"`
}

// ManageTest manages the c2s test lifecycle.
func ManageTest(ctx context.Context, conn protocol.Connection, s ndt.Server) (*ArchivalData, error) {
	localContext, localCancel := context.WithTimeout(ctx, 30*time.Second)
	defer localCancel()
	record := &ArchivalData{}

	srv, err := s.SingleServingServer("c2s")
	if err != nil {
		log.Println("Could not start SingleServingServer", err)
		record.Error = err.Error()
		return record, err
	}

	err = protocol.SendJSONMessage(protocol.TestPrepare, strconv.Itoa(srv.Port()), conn)
	if err != nil {
		log.Println("Could not send TestPrepare", err)
		record.Error = err.Error()
		return record, err
	}

	testConn, err := srv.ServeOnce(localContext)
	if err != nil {
		log.Println("Could not successfully ServeOnce", err)
		record.Error = err.Error()
		return record, err
	}
	defer warnonerror.Close(testConn, "Could not close test connection")
	record.TestConnectionUUID = testConn.UUID()
	record.ServerIP = conn.ServerIP()
	record.ClientIP = conn.ClientIP()

	err = protocol.SendJSONMessage(protocol.TestStart, "", conn)
	if err != nil {
		log.Println("Could not send TestStart", err)
		record.Error = err.Error()
		return record, err
	}

	seconds := float64(10)
	startTime := time.Now()
	record.StartTime = startTime
	endTime := startTime.Add(10 * time.Second)
	errorTime := endTime.Add(5 * time.Second)
	err = testConn.SetReadDeadline(errorTime)
	if err != nil {
		log.Println("Could not set deadline", err)
		record.Error = err.Error()
		return record, err
	}
	byteCount, err := testConn.DrainUntil(endTime)
	record.EndTime = time.Now()
	if err != nil {
		if byteCount == 0 {
			log.Println("Could not drain the test connection", byteCount, err)
			record.Error = err.Error()
			return record, err
		}
		// It is possible for the client to reach 10 seconds slightly before the server does.
		seconds = time.Now().Sub(startTime).Seconds()
		if seconds < 9 {
			log.Printf("C2S test only uploaded for %f seconds\n", seconds)
			record.Error = err.Error()
			return record, err
		} else if seconds > 11 {
			log.Printf("C2S test uploaded-read-loop exited late (%f seconds) because the read stalled. We will continue with the test.\n", seconds)
		} else {
			log.Printf("C2S test had an error after %f seconds, which is within acceptable bounds. We will continue with the test.\n", seconds)
		}
	} else {
		// Empty out the buffer for poorly-behaved clients.
		// TODO: ensure this behavior is required by a unit test.
		testConn.DrainUntil(errorTime)
	}
	throughputValue := 8 * float64(byteCount) / 1000 / 10
	record.MeanThroughputMbps = throughputValue / 1000 // Convert Kbps to Mbps

	err = protocol.SendJSONMessage(protocol.TestMsg, strconv.FormatFloat(throughputValue, 'g', -1, 64), conn)
	if err != nil {
		log.Println("Could not send TestMsg with C2S results", err)
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
