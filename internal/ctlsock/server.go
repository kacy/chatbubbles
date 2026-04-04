package ctlsock

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kacy/imsg-bridge/internal/auth"
	"github.com/kacy/imsg-bridge/internal/store"
)

type Admin interface {
	GeneratePairingCode(ttl time.Duration, scopes []string) (auth.PairingCode, error)
	ListClients() ([]store.Client, error)
	RevokeClient(clientID string) (bool, error)
}

type Status struct {
	ServerName     string         `json:"server_name"`
	Version        string         `json:"version"`
	ImsgVersion    string         `json:"imsg_version"`
	TailscaleIP    string         `json:"tailscale_ip,omitempty"`
	PairHost       string         `json:"pair_host,omitempty"`
	TLSFingerprint string         `json:"tls_fingerprint,omitempty"`
	UptimeSeconds  int64          `json:"uptime_seconds"`
	Clients        []store.Client `json:"clients,omitempty"`
}

type StatusFunc func(context.Context) (Status, error)

type Server struct {
	path   string
	admin  Admin
	status StatusFunc
}

func New(path string, admin Admin, status StatusFunc) *Server {
	return &Server{
		path:   path,
		admin:  admin,
		status: status,
	}
}

func (s *Server) Run(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}

	_ = os.Remove(s.path)

	listener, err := net.Listen("unix", s.path)
	if err != nil {
		return fmt.Errorf("listen unix socket: %w", err)
	}
	defer listener.Close()
	defer os.Remove(s.path)

	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("chmod socket: %w", err)
	}

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}

			return fmt.Errorf("accept control socket: %w", err)
		}

		wg.Add(1)
		go func(conn net.Conn) {
			defer wg.Done()
			defer conn.Close()

			if err := s.handleConn(ctx, conn); err != nil {
				log.Printf("control socket request failed: %v", err)
			}
		}(conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) error {
	var req struct {
		Cmd      string   `json:"cmd"`
		TTL      int      `json:"ttl"`
		Scopes   []string `json:"scopes"`
		ClientID string   `json:"client_id"`
	}

	reader := bufio.NewReader(conn)
	if err := json.NewDecoder(reader).Decode(&req); err != nil {
		return writeResponse(conn, map[string]any{
			"error": "request body must be valid json",
		})
	}

	switch strings.TrimSpace(req.Cmd) {
	case "generate_pairing_code":
		pairing, err := s.admin.GeneratePairingCode(time.Duration(req.TTL)*time.Second, req.Scopes)
		if err != nil {
			return writeResponse(conn, map[string]any{"error": err.Error()})
		}

		return writeResponse(conn, map[string]any{
			"code":       pairing.Code,
			"expires_at": pairing.ExpiresAt.Format(time.RFC3339),
			"scopes":     pairing.Scopes,
		})
	case "status":
		status, err := s.status(ctx)
		if err != nil {
			return writeResponse(conn, map[string]any{"error": err.Error()})
		}

		return writeResponse(conn, status)
	case "list_clients":
		clients, err := s.admin.ListClients()
		if err != nil {
			return writeResponse(conn, map[string]any{"error": err.Error()})
		}

		return writeResponse(conn, map[string]any{"clients": clients})
	case "revoke_client":
		ok, err := s.admin.RevokeClient(req.ClientID)
		if err != nil {
			return writeResponse(conn, map[string]any{"error": err.Error()})
		}

		return writeResponse(conn, map[string]any{"ok": ok})
	default:
		return writeResponse(conn, map[string]any{
			"error": "unknown command",
		})
	}
}

func writeResponse(conn net.Conn, value any) error {
	return json.NewEncoder(conn).Encode(value)
}
