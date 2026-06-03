package slack

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const dedupTTL = 5 * time.Minute

// EventDedup deduplicates Slack events across multiple pods using Redis SETNX.
type EventDedup struct {
	rdb        *redis.Client
	instanceID string
}

// NewEventDedup creates a new event deduplicator.
func NewEventDedup(rdb *redis.Client, instanceID string) *EventDedup {
	return &EventDedup{rdb: rdb, instanceID: instanceID}
}

// TryClaim attempts to claim an event for processing.
// Returns true if this instance should process it, false if another pod already claimed it.
func (d *EventDedup) TryClaim(ctx context.Context, eventTS string) bool {
	key := fmt.Sprintf("slack:event:%s", eventTS)
	ok, err := d.rdb.SetNX(ctx, key, d.instanceID, dedupTTL).Result()
	if err != nil {
		// On Redis error, process anyway to avoid dropped messages
		return true
	}
	return ok
}
