package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/kacy/imsg-bridge/internal/events"
	"github.com/kacy/imsg-bridge/internal/store"
)

type Store interface {
	Load() (store.Config, error)
}

type Dispatcher struct {
	store     Store
	validator *Validator
	client    *http.Client
}

func NewDispatcher(state Store, validator *Validator) *Dispatcher {
	return &Dispatcher{
		store:     state,
		validator: validator,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context, event events.Event) {
	cfg, err := d.store.Load()
	if err != nil {
		log.Printf("load webhooks failed: %v", err)
		return
	}

	for _, hook := range cfg.Webhooks {
		if !subscribedTo(hook.Events, event.Type) {
			continue
		}

		go d.send(ctx, hook, event)
	}
}

func (d *Dispatcher) send(ctx context.Context, hook store.Webhook, event events.Event) {
	if _, err := d.validator.Validate(ctx, hook.URL); err != nil {
		log.Printf("skip webhook %s: %v", hook.ID, err)
		return
	}

	payload := map[string]any{
		"event":     event.Type,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      event.Data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("encode webhook payload failed: %v", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(body))
	if err != nil {
		log.Printf("build webhook request failed: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		log.Printf("webhook dispatch failed for %s: %v", hook.ID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("webhook dispatch failed for %s: status %d", hook.ID, resp.StatusCode)
	}
}

func subscribedTo(events []string, name string) bool {
	for _, event := range events {
		if strings.EqualFold(event, name) {
			return true
		}
	}
	return false
}

func cleanEvents(events []string) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		event = strings.TrimSpace(event)
		if event == "" || slices.Contains(out, event) {
			continue
		}
		out = append(out, event)
	}
	return out
}
