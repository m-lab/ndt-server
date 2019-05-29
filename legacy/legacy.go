package legacy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/m-lab/go/rtx"

	"github.com/m-lab/go/prometheusx"
	"github.com/m-lab/go/warnonerror"

	"github.com/m-lab/ndt-server/legacy/c2s"
	"github.com/m-lab/ndt-server/legacy/meta"
	legacymetrics "github.com/m-lab/ndt-server/legacy/metrics"
	"github.com/m-lab/ndt-server/legacy/ndt"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/s2c"
	"github.com/m-lab/ndt-server/metrics"
)

const (
	cTestC2S    = 2
	cTestS2C    = 4
	cTestStatus = 16
	cTestMETA   = 32
)

// NDTResult is the struct that is serialized as JSON to disk as the archival record of an NDT test.
//
// This struct is dual-purpose. It contains the necessary data to allow joining
// with tcp-info data and traceroute-caller data as well as any other UUID-based
// data. It also contains enough data for interested parties to perform
// lightweight data analysis without needing to join with other tools.
type NDTResult struct {
	// GitShortCommit is the Git commit (short form) of the running server code.
	GitShortCommit string

	// These data members should all be self-describing. In the event of confusion,
	// rename them to add clarity rather than adding a comment.
	ControlChannelUUID string
	Protocol           ndt.ConnectionType
	MessageProtocol    string
	ServerIP           string
	ClientIP           string

	StartTime time.Time
	EndTime   time.Time
	C2S       *c2s.ArchivalData `json:",omitempty"`
	S2C       *s2c.ArchivalData `json:",omitempty"`
	Meta      meta.ArchivalData `json:",omitempty"`
}

// SaveData archives the data to disk.
func SaveData(record *NDTResult, datadir string) {
	if record == nil {
		log.Println("nil record won't be saved")
		return
	}
	dir := path.Join(datadir, record.StartTime.Format("2006/01/02"))
	err := os.MkdirAll(dir, 0777)
	if err != nil {
		log.Printf("Could not create directory %s: %v\n", dir, err)
		return
	}
	file, err := protocol.UUIDToFile(dir, record.ControlChannelUUID)
	if err != nil {
		log.Println("Could not open file:", err)
		return
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	err = enc.Encode(record)
	if err != nil {
		log.Println("Could not encode", record, "to", file.Name())
		return
	}
	log.Println("Wrote", file.Name())
}

func panicMsgToErrType(msg string) string {
	okayWords := map[string]struct{}{
		"Login":           {},
		"ParseInt":        {},
		"SrvQueue":        {},
		"MsgLoginVersion": {},
		"MsgLoginTests":   {},
		"C2S":             {},
		"S2C":             {},
		"MsgResults":      {},
		"MsgLogout":       {},
	}
	words := strings.SplitN(msg, " ", 1)
	if len(words) >= 1 {
		word := words[0]
		if _, ok := okayWords[word]; ok {
			return word
		}
	}
	return "panic"
}

// HandleControlChannel is the "business logic" of an NDT test. It is designed
// to run every test, and to never need to know whether the underlying
// connection is just a TCP socket, a WS connection, or a WSS connection. It
// only needs a connection, and a factory for making single-use servers for
// connections of that same type.
func HandleControlChannel(conn protocol.Connection, s ndt.Server) {
	metrics.ActiveTests.WithLabelValues(string(s.ConnectionType())).Inc()
	defer metrics.ActiveTests.WithLabelValues(string(s.ConnectionType())).Dec()
	defer func(start time.Time) {
		legacymetrics.ControlChannelDuration.WithLabelValues(string(s.ConnectionType())).Observe(
			time.Since(start).Seconds())
	}(time.Now())
	defer func() {
		r := recover()
		if r != nil {
			log.Println("Test failed, but we recovered:", r)
			// All of our panic messages begin with an informative first word.  Use that as a label.
			errType := panicMsgToErrType(fmt.Sprint(r))
			metrics.ErrorCount.WithLabelValues(string(s.ConnectionType()), errType).Inc()
		}
	}()
	handleControlChannel(conn, s)
}
func handleControlChannel(conn protocol.Connection, s ndt.Server) {
	// Nothing should take more than 45 seconds, and exiting this method should
	// cause all resources used by the test to be reclaimed.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	log.Println("Handling connection", conn)
	defer warnonerror.Close(conn, "Could not close "+conn.String())

	record := &NDTResult{
		GitShortCommit:     prometheusx.GitShortCommit,
		StartTime:          time.Now(),
		ControlChannelUUID: conn.UUID(),
		ServerIP:           conn.ServerIP(),
		ClientIP:           conn.ClientIP(),
		Protocol:           s.ConnectionType(),
	}
	defer func() {
		record.EndTime = time.Now()
		SaveData(record, s.DataDir())
	}()

	tests, err := s.LoginCeremony(conn)
	rtx.PanicOnError(err, "Login - error reading JSON message")

	if (tests & cTestStatus) == 0 {
		log.Println("We don't support clients that don't support TestStatus")
		return
	}
	testsToRun := []string{}
	runC2s := (tests & cTestC2S) != 0
	runS2c := (tests & cTestS2C) != 0
	runMeta := (tests & cTestMETA) != 0

	if runC2s {
		testsToRun = append(testsToRun, strconv.Itoa(cTestC2S))
	}
	if runS2c {
		testsToRun = append(testsToRun, strconv.Itoa(cTestS2C))
	}
	if runMeta {
		testsToRun = append(testsToRun, strconv.Itoa(cTestMETA))
	}

	m := conn.Messager()
	record.MessageProtocol = m.Encoding().String()
	rtx.PanicOnError(
		m.SendMessage(protocol.SrvQueue, []byte("0")),
		"SrvQueue - Could not send SrvQueue")
	rtx.PanicOnError(
		m.SendMessage(protocol.MsgLogin, []byte("v5.0-NDTinGO")),
		"MsgLoginVersion - Could not send MsgLogin with version")
	rtx.PanicOnError(
		m.SendMessage(protocol.MsgLogin, []byte(strings.Join(testsToRun, " "))),
		"MsgLoginTests - Could not send MsgLogin with the tests")

	var c2sRate, s2cRate float64
	if runC2s {
		record.C2S, err = c2s.ManageTest(ctx, conn, s)
		rtx.PanicOnError(err, "C2S - Could not run c2s test")
		if record.C2S != nil && record.C2S.MeanThroughputMbps != 0 {
			c2sRate = record.C2S.MeanThroughputMbps
			metrics.TestRate.WithLabelValues("c2s").Observe(c2sRate)
		}
	}
	if runS2c {
		record.S2C, err = s2c.ManageTest(ctx, conn, s)
		rtx.PanicOnError(err, "S2C - Could not run s2c test")
		if record.S2C != nil && record.S2C.MeanThroughputMbps != 0 {
			s2cRate = record.S2C.MeanThroughputMbps
			metrics.TestRate.WithLabelValues("s2c").Observe(s2cRate)
		}
	}
	if runMeta {
		record.Meta, err = meta.ManageTest(ctx, m)
		rtx.PanicOnError(err, "META - Could not run meta test")
	}
	speedMsg := fmt.Sprintf("You uploaded at %.4f and downloaded at %.4f", c2sRate*1000, s2cRate*1000)
	log.Println(speedMsg)
	// For historical reasons, clients expect results in kbps
	rtx.PanicOnError(
		m.SendMessage(protocol.MsgResults, []byte(speedMsg)),
		"MsgResults - Could not send test results message")
	rtx.PanicOnError(
		m.SendMessage(protocol.MsgLogout, []byte{}),
		"MsgLogout - Could not send MsgLogout")
}
