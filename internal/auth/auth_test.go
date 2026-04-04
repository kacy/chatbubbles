package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kacy/imsg-bridge/internal/store"
)

func TestTokenRoundTrip(t *testing.T) {
	identity, err := EnsureIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("ensure identity: %v", err)
	}

	manager := NewTokenManager(identity)
	token, claims, err := manager.Mint("c_123", []string{"read", "send"}, time.Hour)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	manager.now = func() time.Time { return time.Unix(claims.Issued, 0).UTC() }
	verified, err := manager.Verify(token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}

	if verified.Sub != "c_123" {
		t.Fatalf("expected subject, got %q", verified.Sub)
	}
}

func TestPairingFlow(t *testing.T) {
	dataDir := t.TempDir()
	identity, err := EnsureIdentity(dataDir)
	if err != nil {
		t.Fatalf("ensure identity: %v", err)
	}

	service := NewService(store.New(filepath.Join(dataDir, "config.json")), NewTokenManager(identity))

	code, err := service.EnsureBootstrap("test mac")
	if err != nil {
		t.Fatalf("ensure bootstrap: %v", err)
	}

	result, err := service.Pair(context.Background(), code, "pixel", "android")
	if err != nil {
		t.Fatalf("pair client: %v", err)
	}

	if result.ClientID == "" || result.Token == "" {
		t.Fatal("expected token and client id")
	}

	client, claims, err := service.Authenticate(result.Token)
	if err != nil {
		t.Fatalf("authenticate token: %v", err)
	}

	if client.ID != result.ClientID {
		t.Fatalf("expected client id %q, got %q", result.ClientID, client.ID)
	}

	if !service.HasScope(claims, "read") {
		t.Fatal("expected read scope")
	}
}

func TestSessionFlow(t *testing.T) {
	dataDir := t.TempDir()
	identity, err := EnsureIdentity(dataDir)
	if err != nil {
		t.Fatalf("ensure identity: %v", err)
	}

	manager := NewTokenManager(identity)
	service := NewService(store.New(filepath.Join(dataDir, "config.json")), manager)

	code, err := service.EnsureBootstrap("test mac")
	if err != nil {
		t.Fatalf("ensure bootstrap: %v", err)
	}

	paired, err := service.Pair(context.Background(), code, "pixel", "android")
	if err != nil {
		t.Fatalf("pair client: %v", err)
	}

	_, claims, err := service.Authenticate(paired.Token)
	if err != nil {
		t.Fatalf("authenticate approver: %v", err)
	}

	session, err := service.CreateSession(context.Background(), "chrome", "web")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	pending, err := service.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if pending.Status != "pending" {
		t.Fatalf("expected pending session, got %q", pending.Status)
	}

	approved, err := service.ApproveSession(context.Background(), session.ID, claims, []string{"read"})
	if err != nil {
		t.Fatalf("approve session: %v", err)
	}
	if approved.Token == "" || approved.ClientID == "" {
		t.Fatal("expected approved token and client id")
	}

	loaded, err := service.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get approved session: %v", err)
	}
	if loaded.Status != "approved" {
		t.Fatalf("expected approved session, got %q", loaded.Status)
	}
}
