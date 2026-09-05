package tenant

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"coffeesos/internal/db"
	"coffeesos/internal/httpx"
)

const (
	HeaderBrandID = "X-Brand-ID"
	HeaderStoreID = "X-Store-ID"
)

// BrandFrom resolves the brand the request operates on. Brand users are pinned
// to their brand; platform admins choose one via X-Brand-ID. Writes a 400 and
// returns false when no brand can be determined.
func BrandFrom(c *gin.Context) (uuid.UUID, bool) {
	p := MustFrom(c)
	if p.BrandID != nil {
		return *p.BrandID, true
	}
	if p.IsPlatformAdmin() {
		if raw := c.GetHeader(HeaderBrandID); raw != "" {
			id, err := uuid.Parse(raw)
			if err == nil {
				return id, true
			}
			httpx.Fail(c, http.StatusBadRequest, "invalid_brand", HeaderBrandID+" must be a UUID")
			return uuid.Nil, false
		}
	}
	httpx.Fail(c, http.StatusBadRequest, "brand_required", "no brand in scope; platform admins must send "+HeaderBrandID)
	return uuid.Nil, false
}

// StoreFrom resolves the store for POS-style requests. Staff are pinned to a
// store; managers, owners and platform admins pass X-Store-ID, which must
// belong to the caller's brand.
func StoreFrom(c *gin.Context, q *db.Queries) (uuid.UUID, bool) {
	p := MustFrom(c)
	if p.StoreID != nil {
		return *p.StoreID, true
	}
	raw := c.GetHeader(HeaderStoreID)
	if raw == "" {
		httpx.Fail(c, http.StatusBadRequest, "store_required", "no store in scope; send "+HeaderStoreID)
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_store", HeaderStoreID+" must be a UUID")
		return uuid.Nil, false
	}
	store, err := q.GetStoreByID(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, "not_found", "store not found")
		return uuid.Nil, false
	}
	if !p.IsPlatformAdmin() && (p.BrandID == nil || store.BrandID != *p.BrandID) {
		httpx.Fail(c, http.StatusForbidden, "forbidden", "store belongs to another brand")
		return uuid.Nil, false
	}
	return store.ID, true
}
