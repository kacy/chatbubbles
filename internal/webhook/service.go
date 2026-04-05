package webhook

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/kacy/chatbubbles/internal/store"
)

type Service struct {
	store     *store.Store
	validator *Validator
	now       func() time.Time
}

func NewService(state *store.Store, validator *Validator) *Service {
	return &Service{
		store:     state,
		validator: validator,
		now:       func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Create(ctx context.Context, rawURL string, events []string) (store.Webhook, error) {
	parsed, err := s.validator.Validate(ctx, rawURL)
	if err != nil {
		return store.Webhook{}, err
	}

	events = cleanEvents(events)
	if len(events) == 0 {
		return store.Webhook{}, errors.New("webhook events are required")
	}

	hook := store.Webhook{
		ID:        randomID("w"),
		URL:       parsed.String(),
		Events:    slices.Clone(events),
		CreatedAt: s.now(),
	}

	_, err = s.store.Update(func(cfg *store.Config) error {
		cfg.Webhooks = append(cfg.Webhooks, hook)
		return nil
	})
	if err != nil {
		return store.Webhook{}, err
	}

	return hook, nil
}

func (s *Service) List() ([]store.Webhook, error) {
	cfg, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	hooks := make([]store.Webhook, len(cfg.Webhooks))
	copy(hooks, cfg.Webhooks)
	return hooks, nil
}

func (s *Service) Delete(id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
	}

	deleted := false
	_, err := s.store.Update(func(cfg *store.Config) error {
		filtered := cfg.Webhooks[:0]
		for _, hook := range cfg.Webhooks {
			if hook.ID == id {
				deleted = true
				continue
			}
			filtered = append(filtered, hook)
		}
		cfg.Webhooks = filtered
		return nil
	})
	if err != nil {
		return false, err
	}
	return deleted, nil
}

func randomID(prefix string) string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}

	return prefix + "_" + base64.RawURLEncoding.EncodeToString(raw)
}
