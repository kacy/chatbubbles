package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kacy/imsg-bridge/internal/attachment"
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
	}, &stubRunner{
		version: "0.5.0",
	}, events.NewHub(), nil, nil, nil)

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
	server := NewServer(Config{}, &stubRunner{
		chats: []imsg.Chat{{ID: 3, Identifier: "+15551234567"}},
	}, events.NewHub(), nil, nil, nil)

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
	runner := &stubRunner{
		messages: []imsg.Message{{ID: 1, ChatID: 7, Text: "hello"}},
	}
	server := NewServer(Config{}, runner, events.NewHub(), nil, nil, nil)

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

	if !runner.lastListMessagesOpts.Attachments {
		t.Fatalf("expected attachments enabled by default")
	}
}

func TestHandleMessagesCanSkipAttachments(t *testing.T) {
	runner := &stubRunner{
		messages: []imsg.Message{{ID: 1, ChatID: 7, Text: "hello"}},
	}
	server := NewServer(Config{}, runner, events.NewHub(), nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/chats/7/messages?limit=1&attachments=0", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if runner.lastListMessagesOpts.Attachments {
		t.Fatalf("expected attachments to be disabled")
	}
}

func TestHandleMessagesUsesHistoryCache(t *testing.T) {
	runner := &stubRunner{
		messages: []imsg.Message{{ID: 1, ChatID: 7, Text: "hello"}},
	}
	server := NewServer(Config{}, runner, events.NewHub(), nil, nil, nil)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/chats/7/messages?limit=1&attachments=0", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	}

	if runner.listMessagesCalls != 1 {
		t.Fatalf("expected one runner call after cache hit, got %d", runner.listMessagesCalls)
	}
}

func TestHandleMessagesInvalidatesHistoryCacheOnEvent(t *testing.T) {
	hub := events.NewHub()
	runner := &stubRunner{
		messages: []imsg.Message{{ID: 1, ChatID: 7, Text: "hello"}},
	}
	server := NewServer(Config{}, runner, hub, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/chats/7/messages?limit=1&attachments=0", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	hub.Publish(events.Event{
		Type: "new_message",
		Data: imsg.Message{ID: 2, ChatID: 7, Text: "new"},
	})

	time.Sleep(10 * time.Millisecond)

	req = httptest.NewRequest(http.MethodGet, "/v1/chats/7/messages?limit=1&attachments=0", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after invalidation, got %d", rec.Code)
	}

	if runner.listMessagesCalls != 2 {
		t.Fatalf("expected cache invalidation to trigger a second runner call, got %d", runner.listMessagesCalls)
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

	server := NewServer(Config{}, &stubRunner{version: "0.5.0"}, events.NewHub(), service, nil, nil)

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
	server := NewServer(Config{}, &stubRunner{}, events.NewHub(), nil, nil, nil)

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

func TestSendMessageEndpoint(t *testing.T) {
	dataDir := t.TempDir()
	identity, err := auth.EnsureIdentity(dataDir)
	if err != nil {
		t.Fatalf("ensure identity: %v", err)
	}

	service := auth.NewService(store.New(filepath.Join(dataDir, "config.json")), auth.NewTokenManager(identity))
	code, err := service.GeneratePairingCode(2*time.Minute, []string{"send"})
	if err != nil {
		t.Fatalf("generate send code: %v", err)
	}
	pairResult, err := service.Pair(context.Background(), code.Code, "sender", "android")
	if err != nil {
		t.Fatalf("pair sender: %v", err)
	}

	server := NewServer(Config{}, &stubRunner{
		sendResult: imsg.SendMessageResult{
			Status:  "sent",
			To:      "+15551234567",
			Service: "sms",
		},
	}, events.NewHub(), service, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"to":"+15551234567","text":"hi","service":"sms"}`))
	req.Header.Set("Authorization", "Bearer "+pairResult.Token)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}

	var body imsg.SendMessageResult
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.To != "+15551234567" || body.Service != "sms" {
		t.Fatalf("unexpected send response %#v", body)
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

	server := NewServer(Config{}, &stubRunner{}, events.NewHub(), service, hooks, nil)

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

func TestCORSPreflight(t *testing.T) {
	server := NewServer(Config{}, &stubRunner{}, events.NewHub(), nil, nil, nil)

	req := httptest.NewRequest(http.MethodOptions, "/v1/server", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("unexpected allow-origin header %q", got)
	}
}

func TestHandleMessagesDecoratesAttachmentIDs(t *testing.T) {
	service := attachment.NewService([]byte("secret"), filepath.Join(t.TempDir(), "tmp"))
	server := NewServer(Config{}, &stubRunner{
		messages: []imsg.Message{{
			ID:     1,
			ChatID: 7,
			Attachments: []imsg.Attachment{{
				Filename: "photo.jpg",
				MIMEType: "image/jpeg",
				Path:     "/tmp/photo.jpg",
			}},
		}},
	}, events.NewHub(), nil, nil, service)

	req := httptest.NewRequest(http.MethodGet, "/v1/chats/7/messages", nil)
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
	if len(body.Messages) != 1 || len(body.Messages[0].Attachments) != 1 {
		t.Fatalf("expected one attachment, got %#v", body)
	}
	if !strings.HasPrefix(body.Messages[0].Attachments[0].ID, "a_") {
		t.Fatalf("expected opaque attachment id, got %q", body.Messages[0].Attachments[0].ID)
	}
}

func TestAttachmentEndpoints(t *testing.T) {
	dataDir := t.TempDir()
	identity, err := auth.EnsureIdentity(dataDir)
	if err != nil {
		t.Fatalf("ensure identity: %v", err)
	}

	authService := auth.NewService(store.New(filepath.Join(dataDir, "config.json")), auth.NewTokenManager(identity))
	code, err := authService.GeneratePairingCode(2*time.Minute, []string{"read", "attach"})
	if err != nil {
		t.Fatalf("generate attach code: %v", err)
	}
	pairResult, err := authService.Pair(context.Background(), code.Code, "attachment-client", "android")
	if err != nil {
		t.Fatalf("pair attachment client: %v", err)
	}

	attachmentService := attachment.NewService(identity.PrivateKey, filepath.Join(dataDir, "tmp"))
	runner := &stubRunner{
		sendAttachmentResult: imsg.SendAttachmentResult{
			Status:  "sent",
			To:      "+15551234567",
			Service: "auto",
		},
	}
	server := NewServer(Config{}, runner, events.NewHub(), authService, nil, attachmentService)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("to", "+15551234567"); err != nil {
		t.Fatalf("write to field: %v", err)
	}
	if err := writer.WriteField("text", "with attachment"); err != nil {
		t.Fatalf("write text field: %v", err)
	}
	part, err := writer.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("hello from api")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/attachments", &body)
	req.Header.Set("Authorization", "Bearer "+pairResult.Token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	if runner.sentAttachmentReq.To != "+15551234567" {
		t.Fatalf("unexpected attachment recipient %q", runner.sentAttachmentReq.To)
	}
	if runner.sentAttachmentBody != "hello from api" {
		t.Fatalf("unexpected attachment body %q", runner.sentAttachmentBody)
	}

	var uploadResp imsg.SendAttachmentResult
	if err := json.NewDecoder(rec.Body).Decode(&uploadResp); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if uploadResp.Attachment.Filename != "hello.txt" {
		t.Fatalf("expected upload filename hello.txt, got %#v", uploadResp.Attachment)
	}

	path := filepath.Join(dataDir, "fixture.txt")
	if err := os.WriteFile(path, []byte("download me"), 0o600); err != nil {
		t.Fatalf("write fixture attachment: %v", err)
	}

	attachmentID, err := attachmentService.Encode(imsg.Attachment{
		Path:      path,
		Filename:  "fixture.txt",
		MIMEType:  "text/plain; charset=utf-8",
		SizeBytes: int64(len("download me")),
	})
	if err != nil {
		t.Fatalf("encode attachment id: %v", err)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/v1/attachments/"+attachmentID, nil)
	downloadReq.Header.Set("Authorization", "Bearer "+pairResult.Token)
	downloadRec := httptest.NewRecorder()
	server.ServeHTTP(downloadRec, downloadReq)

	if downloadRec.Code != http.StatusOK {
		t.Fatalf("expected 200 download, got %d", downloadRec.Code)
	}
	if got := downloadRec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("unexpected content type %q", got)
	}
	if downloadRec.Body.String() != "download me" {
		t.Fatalf("unexpected download body %q", downloadRec.Body.String())
	}
}

type stubRunner struct {
	version              string
	chats                []imsg.Chat
	messages             []imsg.Message
	listMessagesCalls    int
	lastListMessagesOpts imsg.ListMessagesOptions
	sendResult           imsg.SendMessageResult
	sendAttachmentResult imsg.SendAttachmentResult
	sentAttachmentReq    imsg.SendAttachmentRequest
	sentAttachmentBody   string
}

func (s *stubRunner) Version(context.Context) (string, error) {
	return s.version, nil
}

func (s *stubRunner) ListChats(context.Context, int) ([]imsg.Chat, error) {
	return s.chats, nil
}

func (s *stubRunner) ListMessages(_ context.Context, _ int64, opts imsg.ListMessagesOptions) ([]imsg.Message, error) {
	s.listMessagesCalls++
	s.lastListMessagesOpts = opts
	return s.messages, nil
}

func (s *stubRunner) SendMessage(context.Context, imsg.SendMessageRequest) (imsg.SendMessageResult, error) {
	return s.sendResult, nil
}

func (s *stubRunner) SendAttachment(_ context.Context, req imsg.SendAttachmentRequest) (imsg.SendAttachmentResult, error) {
	s.sentAttachmentReq = req

	body, err := os.ReadFile(req.FilePath)
	if err != nil {
		return imsg.SendAttachmentResult{}, err
	}
	s.sentAttachmentBody = string(body)

	return s.sendAttachmentResult, nil
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
