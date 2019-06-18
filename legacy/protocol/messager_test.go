package protocol

import (
	"testing"
)

func TestAnything(t *testing.T) {
}

func assertJSONMessagerIsMessager(jm *jsonMessager) {
	func(m Messager) {}(jm)
}

func assertTLVMessagerIsMessager(tm *tlvMessager) {
	func(m Messager) {}(tm)
}
