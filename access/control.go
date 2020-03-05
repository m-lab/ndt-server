package access

import (
	"context"
	"net/http"
)

// Controller is the interface that all access control types should implement.
type Controller interface {
	Limit(next http.Handler) http.Handler
}

type monitoringContextIDType struct{}

var monitoringContextIDKey = monitoringContextIDType{}

// SetMonitoring returns a derived context with the given value.
func SetMonitoring(ctx context.Context, value bool) context.Context {
	// Add a context value to pass advisory information to the next handler.
	return context.WithValue(ctx, monitoringContextIDKey, value)
}

// GetMonitoring attempts to extract the monitoring value from the given context.
func GetMonitoring(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	value := ctx.Value(monitoringContextIDKey)
	if value == nil {
		return false
	}
	return value.(bool)
}
