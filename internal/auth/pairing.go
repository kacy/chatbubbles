package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kacy/imsg-bridge/internal/store"
)

var ErrUnauthorized = errors.New("unauthorized")
var ErrInvalidCode = errors.New("invalid code")
var ErrSessionExpired = errors.New("session expired")
var ErrForbidden = errors.New("forbidden")

type Client struct {
	ID     string
	Name   string
	Type   string
	Scopes []string
}

type PairResult struct {
	Token     string
	ClientID  string
	ExpiresAt time.Time
	Scopes    []string
}

type PairingCode struct {
	Code      string
	ExpiresAt time.Time
	Scopes    []string
}

type Session struct {
	ID        string
	Code      string
	Status    string
	Token     string
	ClientID  string
	ExpiresAt time.Time
	Scopes    []string
}

type Service struct {
	store        *store.Store
	tokens       *TokenManager
	defaultTTL   time.Duration
	pairingTTL   time.Duration
	defaultScope []string
	now          func() time.Time
	mu           sync.Mutex
}

func NewService(stateStore *store.Store, tokens *TokenManager) *Service {
	return &Service{
		store:        stateStore,
		tokens:       tokens,
		defaultTTL:   90 * 24 * time.Hour,
		pairingTTL:   2 * time.Minute,
		defaultScope: []string{"read", "send", "attach"},
		now:          func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) CreateSession(_ context.Context, clientName string, clientType string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	clientName = strings.TrimSpace(clientName)
	clientType = strings.TrimSpace(clientType)
	if clientName == "" || clientType == "" {
		return Session{}, errors.New("client_name and client_type are required")
	}

	session := Session{
		ID:        randomID("s"),
		Code:      randomDigits(6),
		Status:    "pending",
		ExpiresAt: s.now().Add(3 * time.Minute),
	}

	_, err := s.store.Update(func(cfg *store.Config) error {
		pruneExpiredCodes(cfg, s.now())
		pruneExpiredSessions(cfg, s.now())
		cfg.PendingSessions = append(cfg.PendingSessions, store.PendingSession{
			ID:         session.ID,
			Code:       session.Code,
			ClientName: clientName,
			ClientType: clientType,
			Status:     "pending",
			ExpiresAt:  session.ExpiresAt,
		})
		return nil
	})
	if err != nil {
		return Session{}, err
	}

	return session, nil
}

func (s *Service) GetSession(_ context.Context, sessionID string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID = strings.TrimSpace(sessionID)
	cfg, err := s.store.Load()
	if err != nil {
		return Session{}, err
	}

	now := s.now()
	for _, pending := range cfg.PendingSessions {
		if pending.ID != sessionID {
			continue
		}
		if !pending.ExpiresAt.After(now) {
			return Session{}, ErrSessionExpired
		}

		return Session{
			ID:        pending.ID,
			Code:      pending.Code,
			Status:    pending.Status,
			Token:     pending.Token,
			ClientID:  pending.ClientID,
			ExpiresAt: pending.ExpiresAt,
			Scopes:    slices.Clone(pending.Scopes),
		}, nil
	}

	return Session{}, errors.New("session not found")
}

func (s *Service) ApproveSession(_ context.Context, sessionID string, approverClaims TokenClaims, scopes []string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID = strings.TrimSpace(sessionID)
	requestedScopes := cleanScopes(scopes)
	if len(requestedScopes) == 0 {
		requestedScopes = []string{"read", "send"}
	}
	if !scopesSubset(requestedScopes, approverClaims.Scopes) {
		return Session{}, ErrForbidden
	}

	var result Session
	_, err := s.store.Update(func(cfg *store.Config) error {
		now := s.now()
		pruneExpiredSessions(cfg, now)

		for i := range cfg.PendingSessions {
			session := &cfg.PendingSessions[i]
			if session.ID != sessionID {
				continue
			}
			if !session.ExpiresAt.After(now) {
				return ErrSessionExpired
			}

			clientID := randomID("c")
			token, claims, err := s.tokens.Mint(clientID, requestedScopes, 24*time.Hour)
			if err != nil {
				return err
			}

			cfg.Clients = append(cfg.Clients, store.Client{
				ID:        clientID,
				Name:      session.ClientName,
				Type:      session.ClientType,
				Scopes:    slices.Clone(requestedScopes),
				TokenJTI:  claims.JTI,
				CreatedAt: now,
				LastSeen:  now,
			})

			session.Status = "approved"
			session.Token = token
			session.ClientID = clientID
			session.Scopes = slices.Clone(requestedScopes)
			session.ExpiresAt = time.Unix(claims.Expiry, 0).UTC()

			result = Session{
				ID:        session.ID,
				Code:      session.Code,
				Status:    session.Status,
				Token:     session.Token,
				ClientID:  session.ClientID,
				ExpiresAt: session.ExpiresAt,
				Scopes:    slices.Clone(session.Scopes),
			}

			return nil
		}

		return errors.New("session not found")
	})
	if err != nil {
		return Session{}, err
	}

	return result, nil
}

func (s *Service) EnsureBootstrap(serverName string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var created string

	_, err := s.store.Update(func(cfg *store.Config) error {
		cfg.ServerName = serverName
		pruneExpiredCodes(cfg, s.now())

		for _, client := range cfg.Clients {
			if !client.Revoked {
				return nil
			}
		}

		if len(cfg.PendingPairingCodes) > 0 {
			created = cfg.PendingPairingCodes[0].Code
			return nil
		}

		created = randomCode(6)
		cfg.PendingPairingCodes = append(cfg.PendingPairingCodes, store.PendingPairCode{
			Code:      created,
			Scopes:    slices.Clone(s.defaultScope),
			ExpiresAt: s.now().Add(s.pairingTTL),
		})
		return nil
	})
	if err != nil {
		return "", err
	}

	return created, nil
}

func (s *Service) GeneratePairingCode(ttl time.Duration, scopes []string) (PairingCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ttl <= 0 {
		ttl = s.pairingTTL
	}

	if len(scopes) == 0 {
		scopes = s.defaultScope
	}

	result := PairingCode{
		Code:      randomCode(6),
		ExpiresAt: s.now().Add(ttl),
		Scopes:    cleanScopes(scopes),
	}

	_, err := s.store.Update(func(cfg *store.Config) error {
		pruneExpiredCodes(cfg, s.now())
		cfg.PendingPairingCodes = append(cfg.PendingPairingCodes, store.PendingPairCode{
			Code:      result.Code,
			Scopes:    slices.Clone(result.Scopes),
			ExpiresAt: result.ExpiresAt,
		})
		return nil
	})
	if err != nil {
		return PairingCode{}, err
	}

	return result, nil
}

func (s *Service) Pair(_ context.Context, code string, clientName string, clientType string) (PairResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	code = strings.ToUpper(strings.TrimSpace(code))
	clientName = strings.TrimSpace(clientName)
	clientType = strings.TrimSpace(clientType)
	if code == "" || clientName == "" || clientType == "" {
		return PairResult{}, ErrInvalidCode
	}

	var result PairResult

	_, err := s.store.Update(func(cfg *store.Config) error {
		now := s.now()
		pruneExpiredCodes(cfg, now)

		index := -1
		for i, pending := range cfg.PendingPairingCodes {
			if pending.Code == code {
				index = i
				break
			}
		}
		if index == -1 {
			return ErrInvalidCode
		}

		pending := cfg.PendingPairingCodes[index]
		cfg.PendingPairingCodes = append(cfg.PendingPairingCodes[:index], cfg.PendingPairingCodes[index+1:]...)

		clientID := randomID("c")
		token, claims, err := s.tokens.Mint(clientID, pending.Scopes, s.defaultTTL)
		if err != nil {
			return err
		}

		cfg.Clients = append(cfg.Clients, store.Client{
			ID:        clientID,
			Name:      clientName,
			Type:      clientType,
			Scopes:    slices.Clone(pending.Scopes),
			TokenJTI:  claims.JTI,
			CreatedAt: now,
			LastSeen:  now,
		})

		result = PairResult{
			Token:     token,
			ClientID:  clientID,
			ExpiresAt: time.Unix(claims.Expiry, 0).UTC(),
			Scopes:    slices.Clone(pending.Scopes),
		}

		return nil
	})
	if err != nil {
		return PairResult{}, err
	}

	return result, nil
}

func (s *Service) Authenticate(token string) (Client, TokenClaims, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	claims, err := s.tokens.Verify(strings.TrimSpace(token))
	if err != nil {
		return Client{}, TokenClaims{}, ErrUnauthorized
	}

	var authed Client

	_, err = s.store.Update(func(cfg *store.Config) error {
		for i := range cfg.Clients {
			client := &cfg.Clients[i]
			if client.ID != claims.Sub || client.TokenJTI != claims.JTI || client.Revoked {
				continue
			}

			client.LastSeen = s.now()
			authed = Client{
				ID:     client.ID,
				Name:   client.Name,
				Type:   client.Type,
				Scopes: slices.Clone(client.Scopes),
			}
			return nil
		}

		return ErrUnauthorized
	})
	if err != nil {
		return Client{}, TokenClaims{}, ErrUnauthorized
	}

	return authed, claims, nil
}

func (s *Service) HasScope(claims TokenClaims, scope string) bool {
	for _, candidate := range claims.Scopes {
		if candidate == scope {
			return true
		}
	}

	return false
}

func (s *Service) ListClients() ([]store.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	clients := make([]store.Client, len(cfg.Clients))
	copy(clients, cfg.Clients)
	return clients, nil
}

func (s *Service) RevokeClient(clientID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return false, nil
	}

	revoked := false
	_, err := s.store.Update(func(cfg *store.Config) error {
		for i := range cfg.Clients {
			if cfg.Clients[i].ID != clientID {
				continue
			}

			cfg.Clients[i].Revoked = true
			revoked = true
			return nil
		}

		return nil
	})
	if err != nil {
		return false, err
	}

	return revoked, nil
}

func pruneExpiredCodes(cfg *store.Config, now time.Time) {
	filtered := cfg.PendingPairingCodes[:0]
	for _, pending := range cfg.PendingPairingCodes {
		if pending.ExpiresAt.After(now) {
			filtered = append(filtered, pending)
		}
	}
	cfg.PendingPairingCodes = filtered
}

func pruneExpiredSessions(cfg *store.Config, now time.Time) {
	filtered := cfg.PendingSessions[:0]
	for _, session := range cfg.PendingSessions {
		if session.ExpiresAt.After(now) || session.Status == "approved" {
			filtered = append(filtered, session)
		}
	}
	cfg.PendingSessions = filtered
}

func randomCode(length int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}

	var builder strings.Builder
	builder.Grow(length)
	for _, value := range raw {
		builder.WriteByte(alphabet[int(value)%len(alphabet)])
	}

	return builder.String()
}

func randomDigits(length int) string {
	const digits = "0123456789"
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}

	var builder strings.Builder
	builder.Grow(length)
	for _, value := range raw {
		builder.WriteByte(digits[int(value)%len(digits)])
	}

	return builder.String()
}

func cleanScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(scopes))
	cleaned := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}

		seen[scope] = struct{}{}
		cleaned = append(cleaned, scope)
	}

	slices.Sort(cleaned)
	return cleaned
}

func scopesSubset(want []string, have []string) bool {
	allowed := make(map[string]struct{}, len(have))
	for _, scope := range have {
		allowed[scope] = struct{}{}
	}

	for _, scope := range want {
		if _, ok := allowed[scope]; !ok {
			return false
		}
	}

	return true
}

func PairingMessage(code string, expiresAt time.Time) string {
	return fmt.Sprintf("bootstrap pairing code: %s (expires %s)", code, expiresAt.Format(time.RFC3339))
}
