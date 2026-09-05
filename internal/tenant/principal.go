// Package tenant carries the identity of the caller (who, which brand, which
// store, which role) through the request context. Every handler reads tenant
// scope from here, never from the request body.
package tenant

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Role levels mirror the `roles` table.
const (
	LevelStaff         = 10
	LevelStoreManager  = 20
	LevelBrandOwner    = 30
	LevelPlatformAdmin = 100
)

const (
	RoleStaff         = "staff"
	RoleStoreManager  = "store_manager"
	RoleBrandOwner    = "brand_owner"
	RolePlatformAdmin = "platform_admin"
)

type Principal struct {
	UserID  uuid.UUID
	BrandID *uuid.UUID // nil for platform admins
	StoreID *uuid.UUID // nil for brand-level users
	Role    string
	Level   int
}

func (p Principal) IsPlatformAdmin() bool { return p.Level >= LevelPlatformAdmin }

const ctxKey = "coffeesos.principal"

func Set(c *gin.Context, p Principal) { c.Set(ctxKey, p) }

// From returns the authenticated principal, if any.
func From(c *gin.Context) (Principal, bool) {
	v, ok := c.Get(ctxKey)
	if !ok {
		return Principal{}, false
	}
	p, ok := v.(Principal)
	return p, ok
}

// MustFrom is for handlers behind the auth middleware.
func MustFrom(c *gin.Context) Principal {
	p, ok := From(c)
	if !ok {
		panic("tenant: principal missing; is the auth middleware installed?")
	}
	return p
}
