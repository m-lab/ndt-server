package access

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/m-lab/go/rtx"

	"github.com/prometheus/procfs"
)

func TestTxController_Limit(t *testing.T) {
	tests := []struct {
		name     string
		limit    uint64
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
			limit:    1,
			current:  2,
			procPath: "testdata/proc-success",
			visited:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			procPath = tt.procPath
			device = "eth0"
			tx, err := NewTxController(tt.limit)
			if !tt.wantErr && (err != nil) {
				t.Errorf("NewTxController() got %v, want %t", err, tt.wantErr)
				return
			}
			tx.limit = tt.limit
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
		limit    uint64
		want     *TxController
		procPath string
		device   string
		wantErr  bool
	}{
		{
			name:     "failure",
			procPath: "testdata/proc-failure",
			device:   "eth0",
			wantErr:  true,
		},
		{
			name:     "failure-nodevfile",
			procPath: "testdata/proc-nodevfile",
			device:   "eth0",
			wantErr:  true,
		},
		{
			name:     "failure-nodevice",
			procPath: "testdata/proc-nodevice",
			device:   "eth0",
			wantErr:  true,
		},
		{
			name:    "failure-nodevice",
			device:  "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device = tt.device
			procPath = tt.procPath
			got, err := NewTxController(tt.limit)
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

func TestTxController_Watch(t *testing.T) {
	tests := []struct {
		name         string
		limit        uint64
		want         *TxController
		procPath     string
		badProc      string
		wantWatchErr bool
	}{
		{
			name:     "success-zero-rate",
			procPath: "testdata/proc-success",
			limit:    0,
		},
		{
			name:         "success-rate",
			procPath:     "testdata/proc-success",
			limit:        1,
			wantWatchErr: true,
		},
		{
			name:         "success-error-reading-proc",
			procPath:     "testdata/proc-success",
			limit:        1,
			badProc:      "testdata/proc-nodevfile",
			wantWatchErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device = "eth0"
			procPath = tt.procPath
			tx, err := NewTxController(tt.limit)
			if err != nil {
				t.Errorf("NewTxController() error = %v, want nil", err)
				return
			}
			if tt.badProc != "" {
				pfs, err := procfs.NewFS(tt.badProc)
				rtx.Must(err, "Failed to allocate procfs for %q", tt.badProc)
				// New used a good path, but we replace the pfs with a bad proc record.
				tx.pfs = pfs
			}
			tx.period = time.Millisecond
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			err = tx.Watch(ctx)
			if (err != nil) != tt.wantWatchErr {
				t.Errorf("Watch() error = %v, wantErr %v", err, tt.wantWatchErr)
				return
			}
		})
	}
}

type fakeListener struct {
	conn   fakeConn
	err    error
	closed int
}

type fakeConn struct {
	net.TCPConn
	closed int
}

func (c *fakeConn) Close() error {
	c.closed++
	return nil
}

func (f *fakeListener) Accept() (net.Conn, error) {
	return &f.conn, f.err
}
func (f *fakeListener) Close() error {
	f.closed++
	return nil
}
func (f *fakeListener) Addr() net.Addr {
	return &net.TCPAddr{}
}

func TestTxController_Accept(t *testing.T) {
	tests := []struct {
		name       string
		l          *fakeListener
		tx         *TxController
		wantClosed int
		wantErr    bool
	}{
		{
			name: "success-accepted",
			l:    &fakeListener{},
			tx: &TxController{
				current: 0,
				limit:   1,
			},
			wantClosed: 0,
		},
		{
			name: "success-rejected",
			l:    &fakeListener{conn: fakeConn{}},
			tx: &TxController{
				current: 2,
				limit:   1,
			},
			wantClosed: 1,
			wantErr:    true,
		},
		{
			name: "success-accept-with-nil-tx",
			l:    &fakeListener{conn: fakeConn{}},
			tx:   nil, // Accept should work even with a nil tx.
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := tt.tx.Accept(tt.l)
			if (err != nil) != tt.wantErr {
				t.Errorf("TxController.Accept() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			fc, ok := conn.(*fakeConn)
			if conn != nil && ok && fc.closed != tt.wantClosed {
				t.Errorf("TxController.Accept() failed to close conn; got %d, want %d", fc.closed, tt.wantClosed)
			}
		})
	}
}
