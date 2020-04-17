// Package logging contains data structures useful to implement logging
// across ndt-server in a Docker friendly way.
package logging

import (
	golog "log"
	"net/http"
	"os"

	"github.com/apex/log"
	"github.com/apex/log/handlers/json"
	"github.com/gorilla/handlers"
)

// Logger is a logger that logs messages on the standard error
// in a structured JSON format, to simplify processing. Emitting logs
// on the standard error is consistent with the standard practices
// when dockerising an Apache or Nginx instance.
var Logger = log.Logger{
	Handler: json.New(os.Stderr),
	Level:   log.DebugLevel,
}

// MakeAccessLogHandler wraps |handler| with another handler that logs
// access to each resource on the standard output. This is consistent with
// the way in which Apache and Nginx are dockerised. We do not emit JSON
// access logs, because access logs are a fairly standard format that
// has been around for a long time now, so better to follow such standard.
func MakeAccessLogHandler(handler http.Handler) http.Handler {
	return handlers.LoggingHandler(golog.Writer(), handler)
}
