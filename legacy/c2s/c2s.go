package c2s

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/singleserving"
)

// ManageTest manages the c2s test lifecycle.
func ManageTest(ctx context.Context, conn protocol.Connection, f singleserving.Factory) (float64, error) {
	localContext, localCancel := context.WithTimeout(ctx, 30*time.Second)
	defer localCancel()

	srv, err := f.SingleServingServer("c2s")
	if err != nil {
		log.Println("Could not start SingleServingServer", err)
		return 0, err
	}

	err = protocol.SendJSONMessage(protocol.TestPrepare, strconv.Itoa(srv.Port()), conn)
	if err != nil {
		log.Println("Could not send TestPrepare", err)
		return 0, err
	}

	testConn, err := srv.ServeOnce(localContext)
	if err != nil {
		log.Println("Could not successfully ServeOnce", err)
		return 0, err
	}
	defer warnonerror.Close(testConn, "Could not close test connection")

	err = protocol.SendJSONMessage(protocol.TestStart, "", conn)
	if err != nil {
		log.Println("Could not send TestStart", err)
		return 0, err
	}

	seconds := float64(10)
	startTime := time.Now()
	endTime := startTime.Add(10 * time.Second)
	errorTime := endTime.Add(5 * time.Second)
	err = testConn.SetReadDeadline(errorTime)
	if err != nil {
		log.Println("Could not set deadline", err)
		return 0, err
	}
	byteCount, err := testConn.DrainUntil(endTime)
	if err != nil {
		if byteCount == 0 {
			log.Println("Could not drain the test connection", byteCount, err)
			return 0, err
		}
		// It is possible for the client to reach 10 seconds slightly before the server does.
		seconds = time.Now().Sub(startTime).Seconds()
		if seconds < 9 {
			log.Printf("C2S test only uploaded for %f seconds\n", seconds)
			return 0, err
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

	err = protocol.SendJSONMessage(protocol.TestMsg, strconv.FormatFloat(throughputValue, 'g', -1, 64), conn)
	if err != nil {
		log.Println("Could not send TestMsg with C2S results", err)
		return 0, err
	}

	err = protocol.SendJSONMessage(protocol.TestFinalize, "", conn)
	if err != nil {
		log.Println("Could not send TestFinalize", err)
		return throughputValue, err
	}

	return throughputValue, nil
}
