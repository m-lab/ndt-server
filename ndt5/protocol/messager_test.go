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
	if len(fm.sentMessages) < 20 {
		t.Error("Bad messages:", fm)
	}
}

func TestSendMetricsWithErrors(t *testing.T) {
	data := &web100.Metrics{}
	fm := &fakeMessager{
		errorAfter: 25,
	}
	err := SendMetrics(data, fm, "")
	if err == nil {
		t.Error("Error should not be nil", err)
	}
	if len(fm.sentMessages) > 30 {
		t.Error("Bad messages:", fm)
	}
}
