package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/kacy/imsg-bridge/internal/auth"
)

type Authenticator interface {
	Authenticate(token string) (auth.Client, auth.TokenClaims, error)
	HasScope(claims auth.TokenClaims, scope string) bool
}

type authContextKey string

const authClientKey authContextKey = "auth-client"
const authClaimsKey authContextKey = "auth-claims"

func (s *Server) requireAuth(scope string, next http.HandlerFunc) http.HandlerFunc {
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

		ctx := context.WithValue(r.Context(), authClientKey, client)
		ctx = context.WithValue(ctx, authClaimsKey, claims)
		next(w, r.WithContext(ctx))
	}
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
