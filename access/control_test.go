package access

import (
	"context"
	"testing"
)

func TestGetMonitoring(t *testing.T) {
	// Set monitoring value true.
	ctx := context.Background()
	ctx = SetMonitoring(ctx, true)
	if m := GetMonitoring(ctx); !m {
		t.Errorf("Set/GetMonitoring() wrong; got %t, want %t", m, true)
	}

	// Set monitoring value false.
	ctx = context.Background()
	ctx = SetMonitoring(ctx, false)
	if m := GetMonitoring(ctx); m {
		t.Errorf("Set/GetMonitoring() wrong; got %t, want %t", m, false)
	}

	// Verify that a nil context WAI.
	if m := GetMonitoring(nil); m {
		t.Errorf("Set/GetMonitoring() wrong; got %t, want %t", m, false)
	}
}
