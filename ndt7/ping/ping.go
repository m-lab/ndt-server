// Package ping implements WebSocket PING messages.
package ping

import (
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
)

// SendTicks sends the current ticks as a ping message.
func SendTicks(conn *websocket.Conn, deadline time.Time) error {
	// TODO(bassosimone): when we'll have a unique base time.Time reference for
	// the whole test, we should use that, since UnixNano() is not monotonic.
	ticks := int64(time.Now().UnixNano())
	data, err := json.Marshal(ticks)
	if err == nil {
		err = conn.WriteControl(websocket.PingMessage, data, deadline)
	}
	return err
}

func ParseTicks(s string) (d int64, err error) {
	// TODO(bassosimone): when we'll have a unique base time.Time reference for
	// the whole test, we should use that, since UnixNano() is not monotonic.
	var prev int64
	err = json.Unmarshal([]byte(s), &prev)
	if err == nil {
		d = (int64(time.Now().UnixNano()) - prev)
	}
	return
}
