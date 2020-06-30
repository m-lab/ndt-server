package c2s

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/ndt5/metrics"
	"github.com/m-lab/ndt-server/ndt5/ndt"
	"github.com/m-lab/ndt-server/ndt5/protocol"
	"github.com/m-lab/ndt-server/ndt5/web100"
)

// ArchivalData is the data saved by the C2S test. If a researcher wants deeper
// data, then they should use the UUID to get deeper data from tcp-info.
type ArchivalData struct {
	// The server and client IP are here as well as in the containing struct
	// because happy eyeballs means that we may have a IPv4 control connection
	// causing a IPv6 connection to the test port or vice versa.
	ServerIP   string
	ServerPort int
	ClientIP   string
	ClientPort int

	// This is the only field that is really required.
	UUID string

	// These fields are here to enable analyses that don't require joining with tcp-info data.
	StartTime          time.Time
	EndTime            time.Time
	MeanThroughputMbps float64
	// TODO: Add TCPEngine (bbr, cubic, reno, etc.)

	Error string `json:",omitempty"`
}

// ManageTest manages the c2s test lifecycle.
func ManageTest(ctx context.Context, controlConn protocol.Connection, s ndt.Server) (record *ArchivalData, err error) {
	localContext, localCancel := context.WithTimeout(ctx, 30*time.Second)
	defer localCancel()
	defer func() {
		if err != nil && record != nil {
			record.Error = err.Error()
		}
	}()
	record = &ArchivalData{}

	m := controlConn.Messager()
	connType := s.ConnectionType().Label()

	srv, err := s.SingleServingServer("c2s")
	if err != nil {
		log.Println("Could not start SingleServingServer", err)
		metrics.ClientTestErrors.WithLabelValues(connType, "c2s", "StartSingleServingServer").Inc()
		return record, err
	}

	err = m.SendMessage(protocol.TestPrepare, []byte(strconv.Itoa(srv.Port())))
	if err != nil {
		log.Println("Could not send TestPrepare", err)
		metrics.ClientTestErrors.WithLabelValues(connType, "c2s", "TestPrepare").Inc()
		return record, err
	}

	testConn, err := srv.ServeOnce(localContext)
	if err != nil {
		log.Println("Could not successfully ServeOnce", err)
		metrics.ClientTestErrors.WithLabelValues(connType, "c2s", "ServeOnce").Inc()
		return record, err
	}

	// When ManageTest exits, close the test connection.
	defer func() {
		// Allow the connection-draining goroutine to empty all buffers in support of
		// poorly-written clients before we close the connection, but do not block the
		// exit of ManageTest on waiting for the test connection to close.
		go func() {
			time.Sleep(3 * time.Second)
			warnonerror.Close(testConn, "Could not close test connection")
		}()
	}()

	record.UUID = testConn.UUID()
	record.ServerIP, record.ServerPort = testConn.ServerIPAndPort()
	record.ClientIP, record.ClientPort = testConn.ClientIPAndPort()

	err = m.SendMessage(protocol.TestStart, []byte{})
	if err != nil {
		log.Println("Could not send TestStart", err, record.UUID)
		metrics.ClientTestErrors.WithLabelValues(connType, "c2s", "TestStart").Inc()
		return record, err
	}

	record.StartTime = time.Now()
	web100Metrics, err := drainForeverButMeasureFor(ctx, testConn, 10*time.Second)
	record.EndTime = time.Now()
	seconds := record.EndTime.Sub(record.StartTime).Seconds()
	log.Println("Ended C2S test on", testConn, record.UUID)
	if err != nil {
		if web100Metrics.TCPInfo.BytesReceived == 0 {
			log.Println("Could not drain the test connection", err, record.UUID)
			metrics.ClientTestErrors.WithLabelValues(connType, "c2s", "Drain").Inc()
			return record, err
		}
		// It is possible for the client to reach 10 seconds slightly before the server does.
		if seconds < 9 {
			log.Printf("C2S test client only uploaded for %f seconds  %s\n", seconds, record.UUID)
			metrics.ClientTestErrors.WithLabelValues(connType, "c2s", "EarlyExit").Inc()
			return record, err
		}
		// More than 9 seconds is fine.
		log.Printf("C2S test had an error (%v) after %f seconds. We will continue with the test.\n", err, seconds)
	}

	throughputValue := 8 * float64(web100Metrics.TCPInfo.BytesReceived) / 1000 / seconds
	record.MeanThroughputMbps = throughputValue / 1000 // Convert Kbps to Mbps

	log.Println(controlConn, "sent us", throughputValue, "Kbps")
	err = m.SendMessage(protocol.TestMsg, []byte(strconv.FormatInt(int64(throughputValue), 10)))
	if err != nil {
		log.Println("Could not send TestMsg with C2S results", err, record.UUID)
		metrics.ClientTestErrors.WithLabelValues(connType, "c2s", "TestMsg").Inc()
		return record, err
	}

	err = m.SendMessage(protocol.TestFinalize, []byte{})
	if err != nil {
		log.Println("Could not send TestFinalize", err, record.UUID)
		metrics.ClientTestErrors.WithLabelValues(connType, "c2s", "TestFinalize").Inc()
		return record, err
	}

	return record, nil
}

// drainForeverButMeasureFor is a generic method for draining a connection while
// measuring the connection for the first part of the drain. This method does
// not close the passed-in Connection, and starts a goroutine which runs until
// that Connection is closed.
func drainForeverButMeasureFor(ctx context.Context, conn protocol.MeasuredConnection, d time.Duration) (*web100.Metrics, error) {
	derivedCtx, derivedCancel := context.WithTimeout(ctx, d)
	defer derivedCancel()

	conn.StartMeasuring(derivedCtx)

	errs := make(chan error, 1)
	// This is the "drain forever" part of this function. Read the passed-in
	// connection until the passed-in connection is closed.
	go func() {
		var connErr error
		// Read the connections until the connection is closed. Reading on a closed
		// connection returns an error, which terminates the loop and the goroutine.
		for connErr == nil {
			_, connErr = conn.ReadBytes()
		}
		errs <- connErr
	}()

	var socketStats *web100.Metrics
	var err error
	select {
	case <-derivedCtx.Done(): // Wait for timeout
		log.Println("Timed out")
		socketStats, err = conn.StopMeasuring()
	case err = <-errs: // Error in c2s transfer
		log.Println("C2S error:", err)
		socketStats, _ = conn.StopMeasuring()
	}
	if socketStats == nil {
		return nil, err
	}
	// socketStats is guaranteed to be non-nil and the TCPInfo element is a value not a pointer.
	return socketStats, err
}
