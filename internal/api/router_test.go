package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kacy/imsg-bridge/internal/events"
	"github.com/kacy/imsg-bridge/internal/imsg"
)

func TestHandleServer(t *testing.T) {
	server := NewServer(Config{
		ServerName:  "test mac",
		Version:     "0.1.0",
		StartedAt:   time.Now().Add(-2 * time.Minute),
		TailscaleIP: "100.64.0.3",
	}, stubRunner{
		version: "0.5.0",
	}, events.NewHub(), nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/server", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Name        string `json:"name"`
		ImsgVersion string `json:"imsg_version"`
		TailscaleIP string `json:"tailscale_ip"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Name != "test mac" {
		t.Fatalf("expected server name, got %q", body.Name)
	}

	if body.ImsgVersion != "0.5.0" {
		t.Fatalf("expected imsg version, got %q", body.ImsgVersion)
	}

	if body.TailscaleIP != "100.64.0.3" {
		t.Fatalf("expected tailscale ip, got %q", body.TailscaleIP)
	}
}

func TestHandleChats(t *testing.T) {
	server := NewServer(Config{}, stubRunner{
		chats: []imsg.Chat{{ID: 3, Identifier: "+15551234567"}},
	}, events.NewHub(), nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/chats?limit=1", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Chats []imsg.Chat `json:"chats"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(body.Chats) != 1 {
		t.Fatalf("expected 1 chat, got %d", len(body.Chats))
	}
}

func TestHandleMessages(t *testing.T) {
	server := NewServer(Config{}, stubRunner{
		messages: []imsg.Message{{ID: 1, ChatID: 7, Text: "hello"}},
	}, events.NewHub(), nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/chats/7/messages?limit=1", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Messages []imsg.Message `json:"messages"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(body.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(body.Messages))
	}
}

type stubRunner struct {
	version  string
	chats    []imsg.Chat
	messages []imsg.Message
}

func (s stubRunner) Version(context.Context) (string, error) {
	return s.version, nil
}

func (s stubRunner) ListChats(context.Context, int) ([]imsg.Chat, error) {
	return s.chats, nil
}

func (s stubRunner) ListMessages(context.Context, int64, imsg.ListMessagesOptions) ([]imsg.Message, error) {
	return s.messages, nil
}
