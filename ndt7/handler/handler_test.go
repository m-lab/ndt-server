// Package handler implements the WebSocket handler for ndt7.
package handler

import (
	"context"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/m-lab/ndt-server/ndt7/download/sender"
	"github.com/m-lab/ndt-server/ndt7/spec"
	"github.com/m-lab/ndt-server/redis"
)

func Test_validateEarlyExit(t *testing.T) {
	type args struct {
		values url.Values
	}
	tests := []struct {
		name    string
		values  url.Values
		want    *sender.Params
		wantErr bool
	}{
		{
			name:   "valid-param",
			values: url.Values{"early_exit": {spec.ValidEarlyExitValues[0]}},
			want: &sender.Params{
				IsEarlyExit: true,
				MaxBytes:    250000000,
			},
			wantErr: false,
		},
		{
			name:    "invalid-param",
			values:  url.Values{"early_exit": {"123"}},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "missing-value",
			values:  url.Values{"early_exit": {""}},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "absent-param",
			values: url.Values{"foo": {"bar"}},
			want: &sender.Params{
				IsEarlyExit: false,
				MaxBytes:    0,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateEarlyExit(tt.values)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEarlyExit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validateEarlyExit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_checkEarlyTermination(t *testing.T) {
	// Note: These tests require a running Redis instance at localhost:6379
	// You can start one with: docker run -d -p 6379:6379 redis:latest
	// Skip these tests if Redis is not available

	redisAddr := "localhost:6379"
	redisClient := redis.NewClient(redisAddr)

	// Try to ping Redis to see if it's available
	ctx := context.Background()
	if err := redisClient.SetTerminationFlag(ctx, "test-ping", 0); err != nil {
		t.Skip("Redis not available, skipping tests. Start Redis with: docker run -d -p 6379:6379 redis:latest")
	}

	t.Run("terminates-when-flag-set-to-1", func(t *testing.T) {
		h := &Handler{
			RedisClient: redisClient,
		}

		uuid := "test-uuid-terminate"
		testCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Track if cancel was called
		var cancelCalled bool
		var mu sync.Mutex

		wrappedCancel := func() {
			mu.Lock()
			cancelCalled = true
			mu.Unlock()
			cancel()
		}

		// Start the monitoring goroutine
		go h.checkEarlyTermination(testCtx, uuid, wrappedCancel)

		// Wait a bit to ensure monitoring starts
		time.Sleep(50 * time.Millisecond)

		// Set the termination flag to 1
		err := redisClient.SetTerminationFlag(context.Background(), uuid, 1)
		if err != nil {
			t.Fatalf("Failed to set termination flag: %v", err)
		}

		// Wait for the context to be cancelled (should happen within ~200ms)
		select {
		case <-testCtx.Done():
			// Success - context was cancelled
			mu.Lock()
			if !cancelCalled {
				t.Error("Context was cancelled but our wrapped cancel function was not called")
			}
			mu.Unlock()
		case <-time.After(500 * time.Millisecond):
			t.Error("checkEarlyTermination did not cancel context when flag was set to 1")
		}

		// Cleanup
		_ = redisClient.SetTerminationFlag(context.Background(), uuid, 0)
	})

	t.Run("continues-when-flag-is-0", func(t *testing.T) {
		h := &Handler{
			RedisClient: redisClient,
		}

		uuid := "test-uuid-continue"
		testCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		// Set the termination flag to 0
		err := redisClient.SetTerminationFlag(context.Background(), uuid, 0)
		if err != nil {
			t.Fatalf("Failed to set termination flag: %v", err)
		}

		// Start the monitoring goroutine
		go h.checkEarlyTermination(testCtx, uuid, cancel)

		// Wait and verify that cancel is NOT called when flag is 0
		time.Sleep(250 * time.Millisecond)

		// The context should only be done due to timeout, not early termination
		select {
		case <-testCtx.Done():
			// This is expected - timeout should occur, not early termination
			if testCtx.Err() != context.DeadlineExceeded {
				t.Errorf("Expected context to timeout, but got error: %v", testCtx.Err())
			}
		}

		// Cleanup
		_ = redisClient.SetTerminationFlag(context.Background(), uuid, 0)
	})

	t.Run("stops-when-context-cancelled", func(t *testing.T) {
		h := &Handler{
			RedisClient: redisClient,
		}

		uuid := "test-uuid-ctx-cancel"
		testCtx, cancel := context.WithCancel(context.Background())

		// Set the termination flag to 0
		err := redisClient.SetTerminationFlag(context.Background(), uuid, 0)
		if err != nil {
			t.Fatalf("Failed to set termination flag: %v", err)
		}

		// Track if the function returns
		done := make(chan bool)
		go func() {
			h.checkEarlyTermination(testCtx, uuid, cancel)
			done <- true
		}()

		// Wait a bit then cancel the context
		time.Sleep(50 * time.Millisecond)
		cancel()

		// Verify the function returns quickly after context cancellation
		select {
		case <-done:
			// Success - function returned
		case <-time.After(200 * time.Millisecond):
			t.Error("checkEarlyTermination did not return after context was cancelled")
		}

		// Cleanup
		_ = redisClient.SetTerminationFlag(context.Background(), uuid, 0)
	})

	t.Run("handles-missing-flag-gracefully", func(t *testing.T) {
		h := &Handler{
			RedisClient: redisClient,
		}

		uuid := "test-uuid-missing-flag"
		testCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		// Don't set any flag - it should default to 0 and not terminate

		// Start the monitoring goroutine
		go h.checkEarlyTermination(testCtx, uuid, cancel)

		// Wait and verify that cancel is NOT called for missing/default flag
		time.Sleep(250 * time.Millisecond)

		// The context should only be done due to timeout
		select {
		case <-testCtx.Done():
			// This is expected - timeout should occur, not early termination
			if testCtx.Err() != context.DeadlineExceeded {
				t.Errorf("Expected context to timeout, but got error: %v", testCtx.Err())
			}
		}
	})

	t.Run("detects-flag-change-from-0-to-1", func(t *testing.T) {
		h := &Handler{
			RedisClient: redisClient,
		}

		uuid := "test-uuid-flag-change"
		testCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Initially set flag to 0
		err := redisClient.SetTerminationFlag(context.Background(), uuid, 0)
		if err != nil {
			t.Fatalf("Failed to set initial termination flag: %v", err)
		}

		// Track if cancel was called
		var cancelCalled bool
		var mu sync.Mutex

		wrappedCancel := func() {
			mu.Lock()
			cancelCalled = true
			mu.Unlock()
			cancel()
		}

		// Start the monitoring goroutine
		go h.checkEarlyTermination(testCtx, uuid, wrappedCancel)

		// Wait a bit to ensure monitoring starts
		time.Sleep(150 * time.Millisecond)

		// Change the flag from 0 to 1
		err = redisClient.SetTerminationFlag(context.Background(), uuid, 1)
		if err != nil {
			t.Fatalf("Failed to update termination flag: %v", err)
		}

		// Wait for the context to be cancelled
		select {
		case <-testCtx.Done():
			// Success - context was cancelled after flag changed
			mu.Lock()
			if !cancelCalled {
				t.Error("Context was cancelled but our wrapped cancel function was not called")
			}
			mu.Unlock()
		case <-time.After(500 * time.Millisecond):
			t.Error("checkEarlyTermination did not detect flag change from 0 to 1")
		}

		// Cleanup
		_ = redisClient.SetTerminationFlag(context.Background(), uuid, 0)
	})
}
