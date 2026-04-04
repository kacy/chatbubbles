package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/kacy/imsg-bridge/internal/store"
)

type WebhookService interface {
	Create(ctx context.Context, rawURL string, events []string) (store.Webhook, error)
	List() ([]store.Webhook, error)
	Delete(id string) (bool, error)
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	if s.webhooks == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "webhooks are not available")
		return
	}

	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "request body must be valid json")
		return
	}

	hook, err := s.webhooks.Create(r.Context(), req.URL, req.Events)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"webhook": hook})
}

func (s *Server) handleListWebhooks(w http.ResponseWriter, _ *http.Request) {
	if s.webhooks == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "webhooks are not available")
		return
	}

	hooks, err := s.webhooks.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"webhooks": hooks})
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	if s.webhooks == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "webhooks are not available")
		return
	}

	ok, err := s.webhooks.Delete(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "webhook was not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
