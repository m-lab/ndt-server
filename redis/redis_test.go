package redis

import (
	"context"
	"testing"
)

func clientSetup(t *testing.T) *Client {
	client := NewClient("localhost:6379")
	// Try to ping Redis to see if it's available
	ctx := context.Background()
	if err := client.SetTerminationFlag(ctx, "test-ping", 0); err != nil {
		t.Skip("Redis not available, skipping tests. Start Redis with: docker run -d -p 6379:6379 redis:latest")
	}
	return client
}

func Test_SetAndGetTerminationFlag(t *testing.T) {
	redisClient := clientSetup(t)
	uuid := "test-uuid-001"
	var flags = [2]int{0, 1}
	for _, flag := range flags {
		err := redisClient.SetTerminationFlag(context.Background(), uuid, flag)
		if err != nil {
			t.Fatalf("Failed to set termination flag: %v", err)
		}
		f, err := redisClient.GetTerminationFlag(context.Background(), uuid)
		if f != flag {
			t.Fatalf("Termination flag set incorrectly: %v instead of %v", f, flag)
		}
		if err != nil {
			t.Fatalf("Failed to get termination flag: %v", err)
		}
	}

	// Cleanup
	_ = redisClient.SetTerminationFlag(context.Background(), uuid, 0)
}
