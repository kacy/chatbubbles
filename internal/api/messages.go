package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/kacy/imsg-bridge/internal/imsg"
)

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		To      string `json:"to"`
		Text    string `json:"text"`
		Service string `json:"service"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "request body must be valid json")
		return
	}

	req.To = strings.TrimSpace(req.To)
	req.Text = strings.TrimSpace(req.Text)
	req.Service = strings.ToLower(strings.TrimSpace(req.Service))
	if req.Service == "" {
		req.Service = "auto"
	}

	if req.To == "" || req.Text == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "to and text are required")
		return
	}

	switch req.Service {
	case "auto", "imessage", "sms":
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "service must be one of auto, imessage, or sms")
		return
	}

	result, err := s.runner.SendMessage(r.Context(), imsg.SendMessageRequest{
		To:      req.To,
		Text:    req.Text,
		Service: req.Service,
	})
	if err != nil {
		writeError(w, statusForErr(err), "internal", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, result)
}
