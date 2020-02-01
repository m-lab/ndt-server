package access

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMaxController_Limit(t *testing.T) {
	tests := []struct {
		name    string
		Max     int64
		Current int64
		want    bool
		status  int
	}{
		{
			name:   "succes",
			Max:    0,
			want:   true,
			status: http.StatusOK,
		},
		{
			name:    "rejected",
			Max:     1,
			Current: 1,
			want:    false,
			status:  http.StatusServiceUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &MaxController{
				Max:     tt.Max,
				Current: tt.Current,
			}
			visited := false
			next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				visited = true
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rw := httptest.NewRecorder()

			c.Limit(next).ServeHTTP(rw, req)

			if visited != tt.want {
				t.Errorf("MaxController.Limit() got %t, want %t", visited, tt.want)
			}
		})
	}
}
