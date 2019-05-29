package meta

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/m-lab/ndt-server/legacy/protocol"
)

// maxClientMessages is the maximum allowed messages we will accept from a client.
var maxClientMessages = 10

// NameValue is an individual meta response.
type NameValue struct {
	Name  string
	Value string
}

// ArchivalData contains all meta data reported by the client.
type ArchivalData []NameValue

type archiveErr struct {
	archivalData ArchivalData
	err          error
}

// ManageTest runs the meta tests. If the ctx is Done before the meta test is
// completed, then the given conn is closed and the context error returned.
func ManageTest(ctx context.Context, m protocol.Messager) (ArchivalData, error) {
	localCtx, localCancel := context.WithTimeout(ctx, 15*time.Second)
	defer localCancel()

	c := make(chan *archiveErr)
	go collectMeta(m, c)
	defer close(c)

	select {
	case <-localCtx.Done():
		return nil, localCtx.Err()
	case ae := <-c:
		return ae.archivalData, ae.err
	}
}

// collectMeta actually collects the meta data from the client and reports
// results over the given channel.
func collectMeta(m protocol.Messager, c chan *archiveErr) {
	var err error
	var message []byte
	results := []NameValue{}

	m.SendMessage(protocol.TestPrepare, []byte{})
	m.SendMessage(protocol.TestStart, []byte{})
	count := 0
	for count < maxClientMessages {
		message, err = m.ReceiveMessage(protocol.TestMsg)
		if string(message) == "" || err != nil {
			break
		}
		count++

		log.Println("Meta message: ", string(message))
		s := strings.SplitN(string(message), ":", 2)
		name := s[0]
		if len(name) > 63 {
			name = name[:63]
		}
		value := s[1]
		if len(value) > 255 {
			value = value[:255]
		}
		results = append(results, NameValue{Name: name, Value: value})
	}
	if err != nil {
		log.Println("Error reading JSON message:", err)
		c <- &archiveErr{archivalData: nil, err: err}
		return
	}
	m.SendMessage(protocol.TestFinalize, []byte{})
	c <- &archiveErr{archivalData: results, err: nil}
}
