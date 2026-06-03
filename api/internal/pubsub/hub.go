package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Hub manages Pub/Sub channels for real-time session events
type Hub struct {
	rdb    *redis.Client
	logger *zap.Logger
}

// NewHub creates a new Pub/Sub hub
func NewHub(rdb *redis.Client, logger *zap.Logger) *Hub {
	return &Hub{
		rdb:    rdb,
		logger: logger,
	}
}

func sessionChannel(sessionID string) string {
	return fmt.Sprintf("session_events:%s", sessionID)
}

func readStateChannel(userEmail string) string {
	return fmt.Sprintf("read_state:%s", userEmail)
}

// Publish sends an event to all subscribers of a session
func (h *Hub) Publish(ctx context.Context, sessionID string, event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	return h.rdb.Publish(ctx, sessionChannel(sessionID), data).Err()
}

// Subscribe returns a channel that receives events for a session.
// The returned cancel function must be called to clean up the subscription.
func (h *Hub) Subscribe(ctx context.Context, sessionID string) (<-chan []byte, func(), error) {
	sub := h.rdb.Subscribe(ctx, sessionChannel(sessionID))

	// Verify subscription
	if _, err := sub.Receive(ctx); err != nil {
		sub.Close()
		return nil, nil, fmt.Errorf("subscribe: %w", err)
	}

	ch := make(chan []byte, 64)

	go func() {
		defer close(ch)
		msgCh := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				select {
				case ch <- []byte(msg.Payload):
				default:
					// Drop if channel full (subscriber too slow)
					h.logger.Warn("dropping pub/sub event: subscriber too slow",
						zap.String("session_id", sessionID))
				}
			}
		}
	}()

	cancel := func() {
		sub.Close()
	}

	return ch, cancel, nil
}

// ReadStateEvent represents a read state change pushed via SSE
type ReadStateEvent struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Count     int    `json:"count"`
}

// PublishReadState sends a read state update to all subscribers of a user
func (h *Hub) PublishReadState(ctx context.Context, userEmail, sessionID string, count int) error {
	event := ReadStateEvent{
		Type:      "read_state_update",
		SessionID: sessionID,
		Count:     count,
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal read state event: %w", err)
	}
	return h.rdb.Publish(ctx, readStateChannel(userEmail), data).Err()
}

// SubscribeReadState returns a channel that receives read state events for a user.
// The returned cancel function must be called to clean up the subscription.
func (h *Hub) SubscribeReadState(ctx context.Context, userEmail string) (<-chan []byte, func(), error) {
	sub := h.rdb.Subscribe(ctx, readStateChannel(userEmail))

	// Verify subscription
	if _, err := sub.Receive(ctx); err != nil {
		sub.Close()
		return nil, nil, fmt.Errorf("subscribe read state: %w", err)
	}

	ch := make(chan []byte, 64)

	go func() {
		defer close(ch)
		msgCh := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				select {
				case ch <- []byte(msg.Payload):
				default:
					h.logger.Warn("dropping read state event: subscriber too slow",
						zap.String("user", userEmail))
				}
			}
		}
	}()

	cancel := func() {
		sub.Close()
	}

	return ch, cancel, nil
}
