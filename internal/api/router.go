package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kacy/imsg-bridge/internal/attachment"
	"github.com/kacy/imsg-bridge/internal/auth"
	"github.com/kacy/imsg-bridge/internal/events"
	"github.com/kacy/imsg-bridge/internal/imsg"
)

type Config struct {
	ListenAddr  string
	ServerName  string
	TailscaleIP string
	Version     string
	StartedAt   time.Time
}

type Runner interface {
	Version(ctx context.Context) (string, error)
	ListChats(ctx context.Context, limit int) ([]imsg.Chat, error)
	ListMessages(ctx context.Context, chatID int64, opts imsg.ListMessagesOptions) ([]imsg.Message, error)
	SendMessage(ctx context.Context, req imsg.SendMessageRequest) (imsg.SendMessageResult, error)
	SendAttachment(ctx context.Context, req imsg.SendAttachmentRequest) (imsg.SendAttachmentResult, error)
}

type Server struct {
	cfg         Config
	auth        Authenticator
	pairing     *auth.Service
	hub         *events.Hub
	limiter     *rateLimiter
	runner      Runner
	webhooks    WebhookService
	attachments *attachment.Service
}

func NewServer(cfg Config, runner Runner, hub *events.Hub, pairing *auth.Service, webhooks WebhookService, attachments *attachment.Service) http.Handler {
	s := &Server{
		cfg:         cfg,
		pairing:     pairing,
		hub:         hub,
		limiter:     newRateLimiter(),
		runner:      runner,
		webhooks:    webhooks,
		attachments: attachments,
	}
	if pairing != nil {
		s.auth = pairing
	}

	if s.cfg.TailscaleIP == "" {
		s.cfg.TailscaleIP = detectTailscaleIP()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/pair", s.limitByIP("pair", 5, time.Minute, s.handlePair))
	mux.HandleFunc("POST /v1/sessions", s.limitByIP("sessions", 5, time.Minute, s.handleCreateSession))
	mux.HandleFunc("GET /v1/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("POST /v1/sessions/{id}/approve", s.requireAuth("send", "send", s.handleApproveSession))
	mux.HandleFunc("POST /v1/webhooks", s.requireAuth("admin", "read", s.handleCreateWebhook))
	mux.HandleFunc("GET /v1/webhooks", s.requireAuth("admin", "read", s.handleListWebhooks))
	mux.HandleFunc("DELETE /v1/webhooks/{id}", s.requireAuth("admin", "read", s.handleDeleteWebhook))
	mux.HandleFunc("GET /v1/server", s.requireAuth("read", "read", s.handleServer))
	mux.HandleFunc("GET /v1/chats", s.requireAuth("read", "read", s.handleChats))
	mux.HandleFunc("GET /v1/chats/{id}/messages", s.requireAuth("read", "read", s.handleMessages))
	mux.HandleFunc("POST /v1/messages", s.requireAuth("send", "send", s.handleSendMessage))
	mux.HandleFunc("POST /v1/attachments", s.requireAuth("attach", "attach", s.handleSendAttachment))
	mux.HandleFunc("GET /v1/attachments/{id}", s.requireAuth("read", "read", s.handleGetAttachment))
	mux.HandleFunc("GET /v1/events", s.requireAuth("read", "read", s.handleEvents))

	return s.logRequests(s.withCORS(mux))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleServer(w http.ResponseWriter, r *http.Request) {
	imsgVersion, err := s.runner.Version(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	resp := struct {
		Name          string `json:"name"`
		Version       string `json:"version"`
		ImsgVersion   string `json:"imsg_version"`
		UptimeSeconds int64  `json:"uptime_seconds"`
		TailscaleIP   string `json:"tailscale_ip,omitempty"`
	}{
		Name:          s.cfg.ServerName,
		Version:       s.cfg.Version,
		ImsgVersion:   imsgVersion,
		UptimeSeconds: int64(time.Since(s.cfg.StartedAt).Seconds()),
		TailscaleIP:   s.cfg.TailscaleIP,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleChats(w http.ResponseWriter, r *http.Request) {
	limit, err := boundedInt(r.URL.Query().Get("limit"), 50, 1, 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	chats, err := s.runner.ListChats(r.Context(), limit)
	if err != nil {
		writeError(w, statusForErr(err), "internal", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string][]imsg.Chat{"chats": chats})
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	chatID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "chat id must be an integer")
		return
	}

	limit, err := boundedInt(r.URL.Query().Get("limit"), 50, 1, 500)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	opts := imsg.ListMessagesOptions{
		Limit:       limit,
		Attachments: true,
	}

	if before := strings.TrimSpace(r.URL.Query().Get("before")); before != "" {
		ts, err := time.Parse(time.RFC3339, before)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "before must be RFC3339")
			return
		}
		opts.Before = &ts
	}

	if after := strings.TrimSpace(r.URL.Query().Get("after")); after != "" {
		ts, err := time.Parse(time.RFC3339, after)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "after must be RFC3339")
			return
		}
		opts.After = &ts
	}

	if opts.Before != nil && opts.After != nil && !opts.After.Before(*opts.Before) {
		writeError(w, http.StatusBadRequest, "bad_request", "after must be earlier than before")
		return
	}

	messages, err := s.runner.ListMessages(r.Context(), chatID, opts)
	if err != nil {
		writeError(w, statusForErr(err), "internal", err.Error())
		return
	}
	if s.attachments != nil {
		s.attachments.DecorateMessages(messages)
	}

	writeJSON(w, http.StatusOK, map[string][]imsg.Message{"messages": messages})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func boundedInt(raw string, fallback, min, max int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("limit must be an integer")
	}

	if value < min || value > max {
		return 0, errors.New("limit is out of range")
	}

	return value, nil
}

func statusForErr(err error) int {
	if errors.Is(err, imsg.ErrUnavailable) {
		return http.StatusServiceUnavailable
	}

	return http.StatusInternalServerError
}

func detectTailscaleIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}

			if ip[0] == 100 {
				return ip.String()
			}
		}
	}

	return ""
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("json encode failed: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
