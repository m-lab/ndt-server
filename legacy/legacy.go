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

	"github.com/m-lab/go/prometheusx"

	"github.com/m-lab/ndt-server/legacy/c2s"
	"github.com/m-lab/ndt-server/legacy/meta"
	legacymetrics "github.com/m-lab/ndt-server/legacy/metrics"
	"github.com/m-lab/ndt-server/legacy/ndt"
	"github.com/m-lab/ndt-server/legacy/protocol"
	"github.com/m-lab/ndt-server/legacy/s2c"
)

const (
	cTestC2S    = 2
	cTestS2C    = 4
	cTestStatus = 16
)

// NDTResult is the struct that is serialized as JSON to disk as the archival record of an NDT test.
//
// This struct is dual-purpose. It contains the necessary data to allow joining
// with tcp-info data and traceroute-caller data as well as any other UUID-based
// data. It also contains enough data for interested parties to perform
// lightweight data analysis without needing to join with other tools.
type NDTResult struct {
	// GitCommit is the Git commit (short form) of the running server code.
	GitCommit string

	// These data members should all be self-describing. In the event of confusion,
	// rename them to add clarity rather than adding a comment.
	ControlChannelUUID string
	Protocol           ndt.ConnectionType
	ServerIP           string
	ClientIP           string

	StartTime time.Time
	EndTime   time.Time
	C2S       *c2s.ArchivalData
	S2C       *s2c.ArchivalData
	Meta      *meta.ArchivalData
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

// HandleControlChannel is the "business logic" of an NDT test. It is designed
// to run every test, and to never need to know whether the underlying
// connection is just a TCP socket, a WS connection, or a WSS connection. It
// only needs a connection, and a factory for making single-use servers for
// connections of that same type.
func HandleControlChannel(conn protocol.Connection, s ndt.Server) {
	// Nothing should take more than 45 seconds, and exiting this method should
	// cause all resources used by the test to be reclaimed.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	record := &NDTResult{
		GitCommit:          prometheusx.GitShortCommit,
		StartTime:          time.Now(),
		ControlChannelUUID: conn.UUID(),
		ServerIP:           conn.ServerIP(),
		ClientIP:           conn.ClientIP(),
		Protocol:           s.ConnectionType(),
	}
	log.Println("Handling connection", *record)
	defer func() {
		record.EndTime = time.Now()
		SaveData(record, s.DataDir())
	}()

	message, err := protocol.ReceiveJSONMessage(conn, protocol.MsgExtendedLogin)
	if err != nil {
		log.Println("Error reading JSON message:", err)
		return
	}
	tests, err := strconv.ParseInt(message.Tests, 10, 64)
	if err != nil {
		log.Println("Failed to parse Tests integer:", err)
		return
	}
	if (tests & cTestStatus) == 0 {
		log.Println("We don't support clients that don't support TestStatus")
		return
	}
	testsToRun := []string{}
	runC2s := (tests & cTestC2S) != 0
	runS2c := (tests & cTestS2C) != 0

	if runC2s {
		testsToRun = append(testsToRun, strconv.Itoa(cTestC2S))
	}
	if runS2c {
		testsToRun = append(testsToRun, strconv.Itoa(cTestS2C))
	}

	protocol.SendJSONMessage(protocol.SrvQueue, "0", conn)
	protocol.SendJSONMessage(protocol.MsgLogin, "v5.0-NDTinGO", conn)
	protocol.SendJSONMessage(protocol.MsgLogin, strings.Join(testsToRun, " "), conn)

	var c2sRate, s2cRate float64
	if runC2s {
		record.C2S, err = c2s.ManageTest(ctx, conn, s)
		if err != nil {
			log.Println("ERROR: manageC2sTest", err)
		}
		if record.C2S != nil && record.C2S.MeanThroughputMbps != 0 {
			c2sRate = record.C2S.MeanThroughputMbps * 1000
			legacymetrics.TestRate.WithLabelValues("c2s").Observe(c2sRate / 1000.0)
		}
	}
	if runS2c {
		record.S2C, err = s2c.ManageTest(ctx, conn, s)
		if err != nil {
			log.Println("ERROR: manageS2cTest", err)
		}
		if record.S2C != nil && record.S2C.MeanThroughputMbps != 0 {
			s2cRate = record.S2C.MeanThroughputMbps * 1000
			legacymetrics.TestRate.WithLabelValues("s2c").Observe(s2cRate / 1000.0)
		}
	}
	log.Printf("NDT: uploaded at %.4f and downloaded at %.4f", c2sRate, s2cRate)
	protocol.SendJSONMessage(protocol.MsgResults, fmt.Sprintf("You uploaded at %.4f and downloaded at %.4f", c2sRate, s2cRate), conn)
	protocol.SendJSONMessage(protocol.MsgLogout, "", conn)
}
