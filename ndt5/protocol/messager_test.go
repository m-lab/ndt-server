package protocol

import (
	"errors"
	"testing"

	"github.com/m-lab/ndt-server/ndt5/web100"
)

func assertJSONMessagerIsMessager(jm *jsonMessager) {
	func(m Messager) {}(jm)
}

func assertTLVMessagerIsMessager(tm *tlvMessager) {
	func(m Messager) {}(tm)
}

type fakeMessager struct {
	sentMessages []string
	errorAfter   int
}

func (fm *fakeMessager) SendMessage(_ MessageType, msg []byte) error {
	fm.sentMessages = append(fm.sentMessages, string(msg))
	if fm.errorAfter > 0 {
		defer func() { fm.errorAfter-- }()
		if fm.errorAfter == 1 {
			return errors.New("Error for testing")
		}
	}
	return nil
}

func (fm *fakeMessager) SendS2CResults(throughputKbps, unsentBytes, totalSentBytes int64) error {
	return nil
}

func (fm *fakeMessager) ReceiveMessage(MessageType) ([]byte, error) { return []byte{}, nil }

func (fm *fakeMessager) Encoding() Encoding {
	return Unknown
}

func TestSendMetrics(t *testing.T) {
	data := &web100.Metrics{}
	fm := &fakeMessager{}
	err := SendMetrics(data, fm, "")
	if err != nil {
		t.Error("Error should be nil", err)
	}
	// 73 was chosen because we needed a number that was greater than zero and not
	// greater than the number of fields in the Metrics struct. This is a moving
	// target, so we don't want to be too specific and require equality with the
	// current count. There were a total of 73 fields as of 2019-08-23, so that's a
	// good lower bound.
	if len(fm.sentMessages) < 73 {
		t.Error("Bad messages:", len(fm.sentMessages), fm)
	}
}

func TestSendMetricsWithErrors(t *testing.T) {
	data := &web100.Metrics{}
	// Erroring after 25 fields means that the error occurs inside the tcpinfo
	// struct, which exercises both error cases in the recursive function.
	fm := &fakeMessager{
		errorAfter: 25,
	}
	err := SendMetrics(data, fm, "")
	if err == nil {
		t.Error("Error should not be nil", err)
	}
	if len(fm.sentMessages) > 25 {
		t.Error("Too many messages sent:", fm)
	}
}
