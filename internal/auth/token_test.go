package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"coffeesos/internal/tenant"
)

func TestTokenRoundTrip(t *testing.T) {
	issuer := NewTokenIssuer("unit-test-secret-0123456789", time.Minute)
	brand := uuid.New()
	store := uuid.New()
	in := tenant.Principal{UserID: uuid.New(), BrandID: &brand, StoreID: &store, Role: tenant.RoleStaff, Level: tenant.LevelStaff}

	raw, exp, err := issuer.Issue(in)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if time.Until(exp) > time.Minute || time.Until(exp) < 50*time.Second {
		t.Fatalf("unexpected expiry %v", exp)
	}
	out, err := issuer.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out.UserID != in.UserID || *out.BrandID != brand || *out.StoreID != store || out.Role != in.Role || out.Level != in.Level {
		t.Fatalf("principal mismatch: %+v vs %+v", out, in)
	}
}

func TestTokenRejectsWrongSecret(t *testing.T) {
	a := NewTokenIssuer("secret-a-0123456789", time.Minute)
	b := NewTokenIssuer("secret-b-0123456789", time.Minute)
	raw, _, _ := a.Issue(tenant.Principal{UserID: uuid.New(), Role: tenant.RolePlatformAdmin, Level: tenant.LevelPlatformAdmin})
	if _, err := b.Parse(raw); err == nil {
		t.Fatal("expected signature error")
	}
}

func TestTokenRejectsExpired(t *testing.T) {
	issuer := NewTokenIssuer("secret-0123456789abcdef", -time.Minute)
	raw, _, _ := issuer.Issue(tenant.Principal{UserID: uuid.New(), Role: tenant.RoleStaff, Level: tenant.LevelStaff})
	if _, err := issuer.Parse(raw); err == nil {
		t.Fatal("expected expiry error")
	}
}
