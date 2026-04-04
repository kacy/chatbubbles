package api

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	tokens     float64
	lastRefill time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		buckets: make(map[string]*bucket),
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (l *rateLimiter) allow(key string, limit int, per time.Duration) (bool, time.Duration) {
	if limit <= 0 || per <= 0 || key == "" {
		return true, 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	ratePerSecond := float64(limit) / per.Seconds()

	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &bucket{
			tokens:     float64(limit - 1),
			lastRefill: now,
		}
		return true, 0
	}

	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens = minFloat(float64(limit), b.tokens+(elapsed*ratePerSecond))
		b.lastRefill = now
	}

	if b.tokens >= 1 {
		b.tokens -= 1
		return true, 0
	}

	need := 1 - b.tokens
	wait := time.Duration(math.Ceil(need/ratePerSecond)) * time.Second
	if wait < time.Second {
		wait = time.Second
	}
	return false, wait
}

func (s *Server) limitByIP(name string, limit int, per time.Duration, next http.HandlerFunc) http.HandlerFunc {
	if s.limiter == nil {
		return next
	}

	return func(w http.ResponseWriter, r *http.Request) {
		key := fmt.Sprintf("ip:%s:%s", name, clientIP(r))
		if ok, retry := s.limiter.allow(key, limit, per); !ok {
			writeRateLimited(w, retry)
			return
		}

		next(w, r)
	}
}

func (s *Server) requireAuth(scope string, action string, next http.HandlerFunc) http.HandlerFunc {
	if s.auth == nil {
		return next
	}

	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" && r.URL.Path == "/v1/events" {
			token = strings.TrimSpace(r.URL.Query().Get("token"))
		}

		client, claims, err := s.auth.Authenticate(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
			return
		}

		if scope != "" && !s.auth.HasScope(claims, scope) {
			writeError(w, http.StatusForbidden, "forbidden", "missing required scope")
			return
		}

		if s.limiter != nil {
			key := fmt.Sprintf("token:%s:%s", action, claims.JTI)
			limit, per := limitForAction(action)
			if ok, retry := s.limiter.allow(key, limit, per); !ok {
				writeRateLimited(w, retry)
				return
			}
		}

		ctx := context.WithValue(r.Context(), authClientKey, client)
		ctx = context.WithValue(ctx, authClaimsKey, claims)
		next(w, r.WithContext(ctx))
	}
}

func limitForAction(action string) (int, time.Duration) {
	switch action {
	case "send":
		return 20, time.Minute
	case "attach":
		return 10, time.Minute
	default:
		return 120, time.Minute
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}

	return strings.TrimSpace(r.RemoteAddr)
}

func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return ""
	}

	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}

	return strings.TrimSpace(token)
}

func writeRateLimited(w http.ResponseWriter, retry time.Duration) {
	if retry > 0 {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(math.Ceil(retry.Seconds()))))
	}
	writeError(w, http.StatusTooManyRequests, "rate_limited", "rate limit exceeded")
}

func minFloat(a float64, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
