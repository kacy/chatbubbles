package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

type Config struct {
	ServerName          string            `json:"server_name,omitempty"`
	Clients             []Client          `json:"clients,omitempty"`
	PendingPairingCodes []PendingPairCode `json:"pending_pairing_codes,omitempty"`
	PendingSessions     []PendingSession  `json:"pending_sessions,omitempty"`
	LastWatchRowID      int64             `json:"last_watch_rowid"`
}

type Client struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Scopes    []string  `json:"scopes"`
	TokenJTI  string    `json:"token_jti"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
	Revoked   bool      `json:"revoked"`
}

type PendingPairCode struct {
	Code      string    `json:"code"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt time.Time `json:"expires_at"`
}

type PendingSession struct {
	ID         string    `json:"id"`
	Code       string    `json:"code"`
	ClientName string    `json:"client_name"`
	ClientType string    `json:"client_type"`
	Status     string    `json:"status"`
	Token      string    `json:"token,omitempty"`
	ClientID   string    `json:"client_id,omitempty"`
	Scopes     []string  `json:"scopes,omitempty"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	return cfg, nil
}

func (s *Store) Save(cfg Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveLocked(cfg)
}

func (s *Store) Update(fn func(*Config) error) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadLocked()
	if err != nil {
		return Config{}, err
	}

	if err := fn(&cfg); err != nil {
		return Config{}, err
	}

	cfg.Clients = slices.Clip(cfg.Clients)
	cfg.PendingPairingCodes = slices.Clip(cfg.PendingPairingCodes)
	cfg.PendingSessions = slices.Clip(cfg.PendingSessions)

	if err := s.saveLocked(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (s *Store) loadLocked() (Config, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	return cfg, nil
}

func (s *Store) saveLocked(cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}

	return nil
}
