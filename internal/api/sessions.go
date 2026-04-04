package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/kacy/imsg-bridge/internal/auth"
)

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if s.pairing == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "sessions are not available")
		return
	}

	var req struct {
		ClientName string `json:"client_name"`
		ClientType string `json:"client_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "request body must be valid json")
		return
	}

	session, err := s.pairing.CreateSession(r.Context(), req.ClientName, req.ClientType)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"session_id": session.ID,
		"code":       session.Code,
		"expires_at": session.ExpiresAt.Format(time.RFC3339),
	})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if s.pairing == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "sessions are not available")
		return
	}

	session, err := s.pairing.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, auth.ErrSessionExpired) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "expired"})
			return
		}

		writeError(w, http.StatusNotFound, "not_found", "session was not found")
		return
	}

	switch session.Status {
	case "approved":
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "approved",
			"token":      session.Token,
			"client_id":  session.ClientID,
			"expires_at": session.ExpiresAt.Format(time.RFC3339),
			"scopes":     session.Scopes,
		})
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "pending"})
	}
}

func (s *Server) handleApproveSession(w http.ResponseWriter, r *http.Request) {
	if s.pairing == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "sessions are not available")
		return
	}

	claims, ok := authClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
		return
	}

	var req struct {
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "request body must be valid json")
		return
	}

	for i := range req.Scopes {
		req.Scopes[i] = strings.TrimSpace(req.Scopes[i])
	}

	session, err := s.pairing.ApproveSession(r.Context(), r.PathValue("id"), claims, req.Scopes)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrSessionExpired):
			writeError(w, http.StatusUnauthorized, "session_expired", "session is expired")
		case errors.Is(err, auth.ErrForbidden):
			writeError(w, http.StatusForbidden, "forbidden", "requested scopes exceed approver scopes")
		default:
			writeError(w, http.StatusNotFound, "not_found", "session was not found")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"client_id":  session.ClientID,
		"expires_at": session.ExpiresAt.Format(time.RFC3339),
	})
}
