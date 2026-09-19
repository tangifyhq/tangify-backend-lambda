package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ably/ably-go/ably"
)

func resetAblyPublisherForTest() {
	ablyOnce = sync.Once{}
	ablyShared = nil
}

func TestNewAblyUtilsDisabledWithoutKey(t *testing.T) {
	t.Setenv("ABLY_KEY", "")
	a, err := NewAblyUtils()
	if err != nil {
		t.Fatalf("NewAblyUtils: %v", err)
	}
	if a.enabled || a.rest != nil {
		t.Fatal("expected disabled REST client when ABLY_KEY is empty")
	}
	if err := a.PublishJSON(context.Background(), "kitchen:test", "order.created", map[string]string{"id": "1"}); err != nil {
		t.Fatalf("disabled publish should be a no-op, got %v", err)
	}
}

func TestNewAblyUtilsUsesREST(t *testing.T) {
	t.Setenv("ABLY_KEY", "abc.def:test-secret")
	a, err := NewAblyUtils()
	if err != nil {
		t.Fatalf("NewAblyUtils: %v", err)
	}
	if !a.enabled || a.rest == nil {
		t.Fatal("expected enabled REST client")
	}
}

func TestAblyPublisherLazyUntilFirstPublish(t *testing.T) {
	t.Setenv("ABLY_KEY", "abc.def:test-secret")
	resetAblyPublisherForTest()
	if ablyShared != nil {
		t.Fatal("publisher must start uninitialized")
	}
	got := ablyPublisher()
	if got == nil || !got.enabled || got.rest == nil {
		t.Fatal("first publish path should create REST client")
	}
	if ablyPublisher() != got {
		t.Fatal("publisher should be reused")
	}
}

func TestPublishJSONPostsOverHTTP(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotBody   []byte
	)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(server.Close)

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}

	client, err := ably.NewREST(
		ably.WithKey("abc.def:test-secret"),
		ably.WithRESTHost(u.Hostname()),
		ably.WithTLSPort(port),
		ably.WithHTTPClient(server.Client()),
		ably.WithUseBinaryProtocol(false),
		ably.WithFallbackHosts([]string{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	a := &AblyUtils{rest: client, enabled: true}
	payload := map[string]string{"id": "ord-1"}
	if err := a.PublishJSON(context.Background(), "kitchen:house", "order.created", payload); err != nil {
		t.Fatalf("PublishJSON: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method %s", gotMethod)
	}
	if gotPath != "/channels/kitchen:house/messages" {
		t.Fatalf("path %s", gotPath)
	}
	var msgs []map[string]any
	if err := json.Unmarshal(gotBody, &msgs); err != nil {
		t.Fatalf("body %s: %v", gotBody, err)
	}
	if len(msgs) != 1 || msgs[0]["name"] != "order.created" {
		t.Fatalf("messages %#v", msgs)
	}
	data, _ := msgs[0]["data"].(string)
	if !strings.Contains(data, `"id":"ord-1"`) {
		t.Fatalf("data %q", data)
	}
}

func TestLoyaltyWaLinkChannels(t *testing.T) {
	t.Setenv("ABLY_CHANNEL", "order_ops")
	t.Setenv("ABLY_CHANNEL_DEV", "order_ops_dev")
	got := loyaltyWaLinkChannels()
	if len(got) != 2 || got[0] != "order_ops" || got[1] != "order_ops_dev" {
		t.Fatalf("got %#v", got)
	}

	t.Setenv("ABLY_CHANNEL_DEV", "order_ops")
	got = loyaltyWaLinkChannels()
	if len(got) != 1 || got[0] != "order_ops" {
		t.Fatalf("duplicate channel: %#v", got)
	}
}

func TestKitchenWaiterChannels(t *testing.T) {
	if kitchenChannel("house") != "kitchen:house" {
		t.Fatal(kitchenChannel("house"))
	}
	if waiterChannel("house") != "waiter:house" {
		t.Fatal(waiterChannel("house"))
	}
}

func TestDisabledPublishLoyaltyWaLink(t *testing.T) {
	a := &AblyUtils{enabled: false}
	if err := a.PublishLoyaltyWaLink(context.Background(), map[string]string{"x": "y"}); err != nil {
		t.Fatalf("got %v", err)
	}
}
