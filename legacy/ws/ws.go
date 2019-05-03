package ws

import (
	"net/http"

	"github.com/gorilla/websocket"
)

// Upgrader returns a struct that can hijack an HTTP(S) connection into a WS(S)
// connection.
func Upgrader(protocol string) *websocket.Upgrader {
	return &websocket.Upgrader{
		ReadBufferSize:    81920,
		WriteBufferSize:   81920,
		Subprotocols:      []string{protocol},
		EnableCompression: false,
		CheckOrigin: func(r *http.Request) bool {
			// TODO: make this check more appropriate -- added to get initial html5 widget to work.
			return true
		},
	}
}
