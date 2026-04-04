package webhook

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/netip"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kacy/imsg-bridge/internal/events"
	"github.com/kacy/imsg-bridge/internal/store"
)

func TestValidatorRejectsPrivateHosts(t *testing.T) {
	v := NewValidator()
	v.lookupIP = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}

	if _, err := v.Validate(context.Background(), "https://example.com/hook"); err == nil {
		t.Fatal("expected private ip validation failure")
	}
}

func TestDispatcherPostsMatchingWebhook(t *testing.T) {
	var mu sync.Mutex
	var calls int

	state := store.New(filepath.Join(t.TempDir(), "config.json"))
	cfg := store.Config{
		Webhooks: []store.Webhook{{
			ID:     "w_1",
			URL:    "https://example.com/hook",
			Events: []string{"new_message"},
		}},
	}
	if err := state.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	validator := NewValidator()
	validator.lookupIP = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}

	dispatcher := NewDispatcher(state, validator)
	dispatcher.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Header:     make(http.Header),
			}, nil
		}),
	}
	dispatcher.Dispatch(context.Background(), events.Event{Type: "new_message", Data: map[string]string{"text": "hi"}})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		seen := calls
		mu.Unlock()
		if seen > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("expected webhook dispatch")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
