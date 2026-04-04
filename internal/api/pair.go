package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/kacy/imsg-bridge/internal/auth"
)

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "pairing is not available")
		return
	}

	var req struct {
		Code       string `json:"code"`
		ClientName string `json:"client_name"`
		ClientType string `json:"client_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "request body must be valid json")
		return
	}

	req.Code = strings.TrimSpace(req.Code)
	req.ClientName = strings.TrimSpace(req.ClientName)
	req.ClientType = strings.TrimSpace(req.ClientType)
	if req.Code == "" || req.ClientName == "" || req.ClientType == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "code, client_name, and client_type are required")
		return
	}

	result, err := s.pairing.Pair(r.Context(), req.Code, req.ClientName, req.ClientType)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCode) {
			writeError(w, http.StatusUnauthorized, "invalid_code", "pairing code is invalid or expired")
			return
		}

		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	resp := struct {
		Token      string   `json:"token"`
		ClientID   string   `json:"client_id"`
		ServerName string   `json:"server_name"`
		ExpiresAt  string   `json:"expires_at"`
		Scopes     []string `json:"scopes"`
	}{
		Token:      result.Token,
		ClientID:   result.ClientID,
		ServerName: s.cfg.ServerName,
		ExpiresAt:  result.ExpiresAt.Format(time.RFC3339),
		Scopes:     result.Scopes,
	}

	writeJSON(w, http.StatusOK, resp)
}
