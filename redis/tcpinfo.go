// tcpinfo.go
// Table 1: TCP-Info Operations

package redis

import (
	"context"
	"encoding/json"
	"time"
)

const (
	table1Prefix = "table_1:"
)

type TCPInfo struct {
	// Define your TCP-Info structure
	UUID string                 `json:"uuid"`
	Data map[string]interface{} `json:"data"`
	// Add specific fields as needed
}

func (c *Client) SetTCPInfo(ctx context.Context, uuid string, info *TCPInfo) error {
	key := table1Prefix + uuid
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, data, time.Hour).Err()
}

func (c *Client) GetTCPInfo(ctx context.Context, uuid string) (*TCPInfo, error) {
	key := table1Prefix + uuid
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var info TCPInfo
	err = json.Unmarshal(data, &info)
	return &info, err
}
