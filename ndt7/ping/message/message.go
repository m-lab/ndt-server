// Package message implements operations with WebSocket PING messages.
package message

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gorilla/websocket"
)

// The json object is used as a namespace to avoid erratic interpretation of
// unsolicited pong frames.  Ping and pong frames are not a part of
// Sec-WebSocket-Protocol, they're part of RFC6455. Section 5.5.3 of the RFC
// allows unsolicited pong frames. Some browsers are known to send unsolicited
// pong frames, see golang/go#6377 <https://github.com/golang/go/issues/6377>.
type pingMessage struct {
	Ndt7TS int64
}

// SendTicks sends the current ticks as a ping message.
func SendTicks(conn *websocket.Conn, start time.Time, deadline time.Time) error {
	msg := pingMessage{
		Ndt7TS: time.Since(start).Nanoseconds(),
	}
	data, err := json.Marshal(msg)
	if err == nil {
		err = conn.WriteControl(websocket.PingMessage, data, deadline)
	}
	return err
}

func ParseTicks(s string, start time.Time) (elapsed time.Duration, d time.Duration, err error) {
	elapsed = time.Since(start)
	var msg pingMessage
	err = json.Unmarshal([]byte(s), &msg)
	if err != nil {
		return
	}
	prev := msg.Ndt7TS
	if 0 <= prev && prev <= elapsed.Nanoseconds() {
		d = time.Duration(elapsed.Nanoseconds() - prev)
	} else {
		err = errors.New("RTT is negative")
	}
	return
}
