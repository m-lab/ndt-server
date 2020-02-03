package access

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestTxController_Limit(t *testing.T) {
	tests := []struct {
		name     string
		rate     uint64
		current  uint64
		procPath string
		visited  bool
		wantErr  bool
	}{
		{
			name:     "success",
			procPath: "testdata/proc-success",
			visited:  true,
		},
		{
			name:     "reject",
			rate:     1,
			current:  2,
			procPath: "testdata/proc-success",
			visited:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			procPath = tt.procPath
			tx, err := NewTxController(tt.rate)
			if !tt.wantErr && (err != nil) {
				t.Errorf("NewTxController() got %v, want %t", err, tt.wantErr)
				return
			}
			tx.rate = tt.rate
			tx.current = tt.current
			visited := false
			next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				visited = true
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rw := httptest.NewRecorder()

			tx.Limit(next).ServeHTTP(rw, req)

			if visited != tt.visited {
				t.Errorf("TxController.Limit() got %t, want %t", visited, tt.visited)
			}
		})
	}
}

func TestNewTxController(t *testing.T) {
	tests := []struct {
		name     string
		rate     uint64
		want     *TxController
		procPath string
		wantErr  bool
	}{
		{
			name:     "failure",
			procPath: "testdata/proc-failure",
			wantErr:  true,
		},
		{
			name:     "failure-nodevfile",
			procPath: "testdata/proc-nodevfile",
			wantErr:  true,
		},
		{
			name:     "failure-nodevice",
			procPath: "testdata/proc-nodevice",
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			procPath = tt.procPath
			got, err := NewTxController(tt.rate)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTxController() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewTxController() = %v, want %v", got, tt.want)
			}
		})
	}
}
