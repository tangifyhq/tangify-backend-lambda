package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/ably/ably-go/ably"
)

type AblyUtils struct {
	rest    *ably.REST
	enabled bool
}

var (
	ablyOnce   sync.Once
	ablyShared *AblyUtils
)

func NewAblyUtils() (*AblyUtils, error) {
	key := os.Getenv("ABLY_KEY")
	if key == "" {
		return &AblyUtils{enabled: false}, nil
	}

	client, err := ably.NewREST(ably.WithKey(key))
	if err != nil {
		return nil, err
	}
	return &AblyUtils{rest: client, enabled: true}, nil
}

// ablyPublisher returns a process-wide REST client, created on first publish.
// Health, menu, and other non-publish routes never call this, so they open
// no Ably connection. REST is HTTP-only; Lambda never subscribes.
func ablyPublisher() *AblyUtils {
	ablyOnce.Do(func() {
		a, err := NewAblyUtils()
		if err != nil {
			fmt.Println("error initializing Ably REST client: ", err)
			ablyShared = &AblyUtils{enabled: false}
			return
		}
		ablyShared = a
	})
	return ablyShared
}

func (a *AblyUtils) PublishJSON(ctx context.Context, channelName string, eventName string, payload any) error {
	if a == nil || !a.enabled {
		return nil
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ch := a.rest.Channels.Get(channelName)
	return ch.Publish(ctx, eventName, string(b))
}

func kitchenChannel(venueID string) string {
	return fmt.Sprintf("kitchen:%s", venueID)
}

func waiterChannel(venueID string) string {
	return fmt.Sprintf("waiter:%s", venueID)
}

func orderOpsChannel() string {
	if ch := strings.TrimSpace(os.Getenv("ABLY_CHANNEL")); ch != "" {
		return ch
	}
	return "order_ops"
}

// loyaltyWaLinkChannels: prod channel always; optional ABLY_CHANNEL_DEV for local dev POS.
func loyaltyWaLinkChannels() []string {
	primary := orderOpsChannel()
	out := []string{primary}
	if dev := strings.TrimSpace(os.Getenv("ABLY_CHANNEL_DEV")); dev != "" && dev != primary {
		out = append(out, dev)
	}
	return out
}

func (a *AblyUtils) PublishLoyaltyWaLink(ctx context.Context, payload any) error {
	if a == nil || !a.enabled {
		return nil
	}
	var firstErr error
	for _, ch := range loyaltyWaLinkChannels() {
		if err := a.PublishJSON(ctx, ch, "loyalty:wa-link", payload); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
