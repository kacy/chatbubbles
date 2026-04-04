package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type TokenClaims struct {
	JTI    string   `json:"jti"`
	Sub    string   `json:"sub"`
	Scopes []string `json:"scopes"`
	Issued int64    `json:"iat"`
	Expiry int64    `json:"exp"`
}

type TokenManager struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	now        func() time.Time
}

func NewTokenManager(identity Identity) *TokenManager {
	return &TokenManager{
		privateKey: identity.PrivateKey,
		publicKey:  identity.PublicKey,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (m *TokenManager) Mint(clientID string, scopes []string, ttl time.Duration) (string, TokenClaims, error) {
	now := m.now()

	claims := TokenClaims{
		JTI:    randomID("t"),
		Sub:    clientID,
		Scopes: cleanScopes(scopes),
		Issued: now.Unix(),
		Expiry: now.Add(ttl).Unix(),
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", TokenClaims{}, fmt.Errorf("encode token claims: %w", err)
	}

	signature := ed25519.Sign(m.privateKey, payload)

	token := base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature)
	return token, claims, nil
}

func (m *TokenManager) Verify(token string) (TokenClaims, error) {
	payloadPart, sigPart, ok := strings.Cut(token, ".")
	if !ok || payloadPart == "" || sigPart == "" {
		return TokenClaims{}, errors.New("token format is invalid")
	}

	payload, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return TokenClaims{}, errors.New("token payload is invalid")
	}

	signature, err := base64.RawURLEncoding.DecodeString(sigPart)
	if err != nil {
		return TokenClaims{}, errors.New("token signature is invalid")
	}

	if !ed25519.Verify(m.publicKey, payload, signature) {
		return TokenClaims{}, errors.New("token signature did not verify")
	}

	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return TokenClaims{}, errors.New("token claims are invalid")
	}

	if m.now().Unix() >= claims.Expiry {
		return TokenClaims{}, errors.New("token is expired")
	}

	return claims, nil
}

func randomID(prefix string) string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}

	return prefix + "_" + base64.RawURLEncoding.EncodeToString(raw)
}
