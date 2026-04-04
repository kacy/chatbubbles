package api

import (
	"context"

	"github.com/kacy/imsg-bridge/internal/auth"
)

type Authenticator interface {
	Authenticate(token string) (auth.Client, auth.TokenClaims, error)
	HasScope(claims auth.TokenClaims, scope string) bool
}

type authContextKey string

const authClientKey authContextKey = "auth-client"
const authClaimsKey authContextKey = "auth-claims"

func authClaimsFromContext(ctx context.Context) (auth.TokenClaims, bool) {
	claims, ok := ctx.Value(authClaimsKey).(auth.TokenClaims)
	return claims, ok
}
