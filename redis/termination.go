// termination.go
// Table 2: Termination Decision Operations

package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	table2Prefix = "table_2:"
)

// SetTerminationFlag sets 0 or 1 for a UUID
func (c *Client) SetTerminationFlag(ctx context.Context, uuid string, flag int) error {
	key := table2Prefix + uuid
	// Optional: set expiration (e.g., 1 hour)
	return c.rdb.Set(ctx, key, flag, time.Hour).Err()
}

// GetTerminationFlag returns 0 or 1, defaults to 0 if not found
func (c *Client) GetTerminationFlag(ctx context.Context, uuid string) (int, error) {
	key := table2Prefix + uuid
	val, err := c.rdb.Get(ctx, key).Int()
	if err == redis.Nil {
		return 0, nil // Default to 0
	}
	return val, err
}
