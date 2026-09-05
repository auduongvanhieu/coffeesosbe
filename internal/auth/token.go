// Package auth issues and verifies JWTs and exposes the login endpoints.
// The token carries tenant scope (brand, store, role) so every downstream
// handler can filter data without touching the users table.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"coffeesos/internal/tenant"
)

const issuerName = "coffeesos"

type Claims struct {
	BrandID *uuid.UUID `json:"bid,omitempty"`
	StoreID *uuid.UUID `json:"sid,omitempty"`
	Role    string     `json:"role"`
	Level   int        `json:"lvl"`
	jwt.RegisteredClaims
}

type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
}

func NewTokenIssuer(secret string, ttl time.Duration) *TokenIssuer {
	return &TokenIssuer{secret: []byte(secret), ttl: ttl}
}

// Issue signs a token for the principal and returns it with its expiry.
func (t *TokenIssuer) Issue(p tenant.Principal) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(t.ttl)
	claims := Claims{
		BrandID: p.BrandID,
		StoreID: p.StoreID,
		Role:    p.Role,
		Level:   p.Level,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuerName,
			Subject:   p.UserID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, exp, nil
}

// Parse verifies the signature and expiry and rebuilds the principal.
func (t *TokenIssuer) Parse(raw string) (tenant.Principal, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(raw, &claims, func(tok *jwt.Token) (any, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return t.secret, nil
	}, jwt.WithIssuer(issuerName), jwt.WithExpirationRequired())
	if err != nil {
		return tenant.Principal{}, err
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return tenant.Principal{}, fmt.Errorf("invalid subject: %w", err)
	}
	return tenant.Principal{
		UserID:  userID,
		BrandID: claims.BrandID,
		StoreID: claims.StoreID,
		Role:    claims.Role,
		Level:   claims.Level,
	}, nil
}
