package c2s

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/m-lab/go/warnonerror"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/singleserving"
	"github.com/m-lab/ndt-server/legacy/testresponder"
)

const (
	ready = float64(-1)
)

// Responder responds to c2s tests.
type Responder struct {
	testresponder.TestResponder
	Response chan float64
}

// TestServer performs the NDT c2s test.
func (tr *Responder) TestServer(w http.ResponseWriter, r *http.Request) {
	upgrader := testresponder.MakeNdtUpgrader([]string{"c2s"})
	wsc, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade should have already returned an HTTP error code.
		log.Println("ERROR C2S: upgrader", err)
		return
	}
	ws := protocol.AdaptWsConn(wsc)
	tr.performTest(ws)
}

func (tr *Responder) performTest(ws protocol.MeasuredConnection) {
	tr.Response <- ready
	bytesPerSecond := tr.recvC2SUntil(ws)
	tr.Response <- bytesPerSecond
	go func() {
		// After the test is supposedly over, let the socket drain a bit to not
		// confuse poorly-written clients by closing unexpectedly when there is still
		// buffered data. We make the judgement call that if the clients are so poorly
		// written that they still have data buffered after 5 seconds and are confused
		// when the c2s socket closes when buffered data is still in flight, then it
		// is okay to break them.
		ws.DrainUntil(time.Now().Add(5 * time.Second))
		ws.Close()
	}()
}

func (tr *Responder) recvC2SUntil(ws protocol.Connection) float64 {
	done := make(chan float64)

	go func() {
		startTime := time.Now()
		endTime := startTime.Add(10 * time.Second)
		totalBytes, err := ws.DrainUntil(endTime)
		if err != nil {
			tr.Close()
			return
		}
		bytesPerSecond := float64(totalBytes) / float64(time.Since(startTime)/time.Second)
		done <- bytesPerSecond
	}()

	log.Println("C2S: Waiting for test to complete or timeout")
	select {
	case <-tr.Ctx.Done():
		log.Println("C2S: Context Done!", tr.Ctx.Err())
		ws.Close()
		// Return zero on error.
		return 0
	case bytesPerSecond := <-done:
		return bytesPerSecond
	}
}

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

	byteCount, err := testConn.DrainUntil(time.Now().Add(10 * time.Second))
	if err != nil {
		log.Println("Could not drain the test connection", err)
		return 0, err
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
