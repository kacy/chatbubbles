package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kacy/imsg-bridge/internal/auth"
	"github.com/kacy/imsg-bridge/internal/events"
	"github.com/kacy/imsg-bridge/internal/imsg"
	"github.com/kacy/imsg-bridge/internal/store"
)

func TestHandleServer(t *testing.T) {
	server := NewServer(Config{
		ServerName:  "test mac",
		Version:     "0.1.0",
		StartedAt:   time.Now().Add(-2 * time.Minute),
		TailscaleIP: "100.64.0.3",
	}, stubRunner{
		version: "0.5.0",
	}, events.NewHub(), nil, nil)

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
	}, events.NewHub(), nil, nil)

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
	}, events.NewHub(), nil, nil)

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

func TestSessionEndpoints(t *testing.T) {
	dataDir := t.TempDir()
	identity, err := auth.EnsureIdentity(dataDir)
	if err != nil {
		t.Fatalf("ensure identity: %v", err)
	}

	service := auth.NewService(store.New(filepath.Join(dataDir, "config.json")), auth.NewTokenManager(identity))
	code, err := service.EnsureBootstrap("test mac")
	if err != nil {
		t.Fatalf("ensure bootstrap: %v", err)
	}
	pairResult, err := service.Pair(context.Background(), code, "pixel", "android")
	if err != nil {
		t.Fatalf("pair client: %v", err)
	}

	server := NewServer(Config{}, stubRunner{version: "0.5.0"}, events.NewHub(), service, nil)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"client_name":"Chrome","client_type":"web"}`))
	createRec := httptest.NewRecorder()
	server.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", createRec.Code)
	}

	var created struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	pollReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+created.SessionID, nil)
	pollRec := httptest.NewRecorder()
	server.ServeHTTP(pollRec, pollReq)
	if pollRec.Code != http.StatusOK {
		t.Fatalf("expected 200 polling, got %d", pollRec.Code)
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+created.SessionID+"/approve", strings.NewReader(`{"scopes":["read"]}`))
	approveReq.Header.Set("Authorization", "Bearer "+pairResult.Token)
	approveRec := httptest.NewRecorder()
	server.ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusOK {
		t.Fatalf("expected 200 approving, got %d", approveRec.Code)
	}

	finalReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+created.SessionID, nil)
	finalRec := httptest.NewRecorder()
	server.ServeHTTP(finalRec, finalReq)
	if finalRec.Code != http.StatusOK {
		t.Fatalf("expected 200 approved poll, got %d", finalRec.Code)
	}

	var approved struct {
		Status string `json:"status"`
		Token  string `json:"token"`
	}
	if err := json.NewDecoder(finalRec.Body).Decode(&approved); err != nil {
		t.Fatalf("decode final response: %v", err)
	}
	if approved.Status != "approved" || approved.Token == "" {
		t.Fatalf("expected approved token response, got %#v", approved)
	}
}

func TestPairEndpointRateLimited(t *testing.T) {
	server := NewServer(Config{}, stubRunner{}, events.NewHub(), nil, nil)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/pair", strings.NewReader(`{"code":"X","client_name":"n","client_type":"android"}`))
		req.RemoteAddr = "100.64.0.2:1234"
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/pair", strings.NewReader(`{"code":"X","client_name":"n","client_type":"android"}`))
	req.RemoteAddr = "100.64.0.2:1234"
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

func TestWebhookEndpoints(t *testing.T) {
	dataDir := t.TempDir()
	identity, err := auth.EnsureIdentity(dataDir)
	if err != nil {
		t.Fatalf("ensure identity: %v", err)
	}

	service := auth.NewService(store.New(filepath.Join(dataDir, "config.json")), auth.NewTokenManager(identity))
	adminCode, err := service.GeneratePairingCode(2*time.Minute, []string{"admin", "read", "send"})
	if err != nil {
		t.Fatalf("generate admin code: %v", err)
	}
	pairResult, err := service.Pair(context.Background(), adminCode.Code, "admin", "android")
	if err != nil {
		t.Fatalf("pair admin client: %v", err)
	}

	hooks := &stubWebhookService{
		created: store.Webhook{
			ID:     "w_123",
			URL:    "https://example.com/hook",
			Events: []string{"new_message"},
		},
		listed: []store.Webhook{{
			ID:     "w_123",
			URL:    "https://example.com/hook",
			Events: []string{"new_message"},
		}},
	}

	server := NewServer(Config{}, stubRunner{}, events.NewHub(), service, hooks)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/webhooks", strings.NewReader(`{"url":"https://example.com/hook","events":["new_message"]}`))
	createReq.Header.Set("Authorization", "Bearer "+pairResult.Token)
	createRec := httptest.NewRecorder()
	server.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200 creating webhook, got %d", createRec.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/webhooks", nil)
	listReq.Header.Set("Authorization", "Bearer "+pairResult.Token)
	listRec := httptest.NewRecorder()
	server.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 listing webhooks, got %d", listRec.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/webhooks/w_123", nil)
	deleteReq.Header.Set("Authorization", "Bearer "+pairResult.Token)
	deleteRec := httptest.NewRecorder()
	server.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting webhook, got %d", deleteRec.Code)
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

type stubWebhookService struct {
	created store.Webhook
	listed  []store.Webhook
}

func (s *stubWebhookService) Create(context.Context, string, []string) (store.Webhook, error) {
	return s.created, nil
}

func (s *stubWebhookService) List() ([]store.Webhook, error) {
	return s.listed, nil
}

func (s *stubWebhookService) Delete(string) (bool, error) {
	return true, nil
}
