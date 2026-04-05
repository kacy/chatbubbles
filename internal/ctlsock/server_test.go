package ctlsock

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/kacy/chatbubbles/internal/auth"
	"github.com/kacy/chatbubbles/internal/store"
)

func TestControlSocketGeneratePairingCode(t *testing.T) {
	server := New("", stubAdmin{}, func(context.Context) (Status, error) {
		return Status{ServerName: "test"}, nil
	})

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	done := make(chan error, 1)
	go func() {
		done <- server.handleConn(context.Background(), serverConn)
	}()

	if err := json.NewEncoder(clientConn).Encode(map[string]any{
		"cmd":    "generate_pairing_code",
		"ttl":    120,
		"scopes": []string{"read"},
	}); err != nil {
		t.Fatalf("encode request: %v", err)
	}

	var resp struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(clientConn).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Code == "" {
		t.Fatal("expected pairing code")
	}

	if err := <-done; err != nil {
		t.Fatalf("handle conn: %v", err)
	}
}

type stubAdmin struct{}

func (stubAdmin) GeneratePairingCode(ttl time.Duration, scopes []string) (auth.PairingCode, error) {
	return auth.PairingCode{Code: "A1B2C3", ExpiresAt: time.Now().UTC().Add(ttl), Scopes: scopes}, nil
}

func (stubAdmin) ListClients() ([]store.Client, error) {
	return []store.Client{{ID: "c_123"}}, nil
}

func (stubAdmin) RevokeClient(string) (bool, error) {
	return true, nil
}
