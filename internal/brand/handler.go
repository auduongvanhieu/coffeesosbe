// Package brand exposes tenant administration: brands and stores (platform
// level) and the current brand's stores and users (brand level).
package brand

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"coffeesos/internal/auth"
	"coffeesos/internal/db"
	"coffeesos/internal/httpx"
	"coffeesos/internal/tenant"
)

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Handler struct {
	q *db.Queries
}

func NewHandler(q *db.Queries) *Handler { return &Handler{q: q} }

// --- Platform (super-admin) ---

func (h *Handler) ListBrands(c *gin.Context) {
	rows, err := h.q.ListBrands(c.Request.Context())
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	if rows == nil {
		rows = []db.Brand{}
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

type createBrandInput struct {
	Slug    string         `json:"slug" binding:"required,max=48"`
	Name    string         `json:"name" binding:"required,max=128"`
	Slogan  *string        `json:"slogan"`
	LogoURL *string        `json:"logoUrl" binding:"omitempty,url"`
	Theme   map[string]any `json:"theme"`
}

func (h *Handler) CreateBrand(c *gin.Context) {
	var in createBrandInput
	if !httpx.Bind(c, &in) {
		return
	}
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	if !slugRe.MatchString(in.Slug) {
		httpx.Fail(c, http.StatusBadRequest, "invalid_slug", "slug must be lowercase letters, digits and hyphens")
		return
	}
	if in.Theme == nil {
		in.Theme = map[string]any{}
	}
	theme, _ := json.Marshal(in.Theme)
	row, err := h.q.CreateBrand(c.Request.Context(), db.CreateBrandParams{
		Slug: in.Slug, Name: in.Name, Slogan: in.Slogan, LogoUrl: in.LogoURL, Theme: theme,
	})
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusCreated, row)
}

// CreateStoreForBrand handles POST /platform/brands/:brandId/stores.
func (h *Handler) CreateStoreForBrand(c *gin.Context) {
	brandID, err := uuid.Parse(c.Param("brandId"))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_id", "brand id must be a UUID")
		return
	}
	h.createStore(c, brandID)
}

// CreateUserForBrand handles POST /platform/brands/:brandId/users.
func (h *Handler) CreateUserForBrand(c *gin.Context) {
	brandID, err := uuid.Parse(c.Param("brandId"))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_id", "brand id must be a UUID")
		return
	}
	h.createUser(c, brandID)
}

// --- Brand admin ---

func (h *Handler) CurrentBrand(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	row, err := h.q.GetBrandByID(c.Request.Context(), brandID)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}

func (h *Handler) ListStores(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	rows, err := h.q.ListStoresByBrand(c.Request.Context(), brandID)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	if rows == nil {
		rows = []db.Store{}
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

func (h *Handler) CreateStore(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	h.createStore(c, brandID)
}

func (h *Handler) ListUsers(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	rows, err := h.q.ListUsersByBrand(c.Request.Context(), &brandID)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	out := make([]auth.UserView, 0, len(rows))
	for _, u := range rows {
		out = append(out, auth.UserView{ID: u.ID, BrandID: u.BrandID, StoreID: u.StoreID, Role: u.RoleCode, Email: u.Email, FullName: u.FullName})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handler) CreateUser(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	h.createUser(c, brandID)
}

// --- shared ---

type createStoreInput struct {
	Code    string  `json:"code" binding:"required,max=32"`
	Name    string  `json:"name" binding:"required,max=128"`
	Address *string `json:"address"`
	Phone   *string `json:"phone"`
}

func (h *Handler) createStore(c *gin.Context, brandID uuid.UUID) {
	var in createStoreInput
	if !httpx.Bind(c, &in) {
		return
	}
	row, err := h.q.CreateStore(c.Request.Context(), db.CreateStoreParams{
		BrandID: brandID, Code: strings.ToLower(strings.TrimSpace(in.Code)), Name: in.Name, Address: in.Address, Phone: in.Phone,
	})
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusCreated, row)
}

type createUserInput struct {
	Email    string     `json:"email" binding:"required,email"`
	Password string     `json:"password" binding:"required,min=8,max=128"`
	FullName string     `json:"fullName" binding:"required,max=128"`
	Phone    *string    `json:"phone"`
	Role     string     `json:"role" binding:"required,oneof=staff store_manager brand_owner"`
	StoreID  *uuid.UUID `json:"storeId"`
}

var roleLevels = map[string]int{
	tenant.RoleStaff:        tenant.LevelStaff,
	tenant.RoleStoreManager: tenant.LevelStoreManager,
	tenant.RoleBrandOwner:   tenant.LevelBrandOwner,
}

// createUser creates a brand-scoped user. A caller may only grant roles below
// their own level unless they are a platform admin.
func (h *Handler) createUser(c *gin.Context, brandID uuid.UUID) {
	var in createUserInput
	if !httpx.Bind(c, &in) {
		return
	}
	p := tenant.MustFrom(c)
	if !p.IsPlatformAdmin() && roleLevels[in.Role] >= p.Level {
		httpx.Fail(c, http.StatusForbidden, "forbidden", "cannot grant a role at or above your own")
		return
	}
	if in.Role == tenant.RoleStaff && in.StoreID == nil {
		httpx.Fail(c, http.StatusBadRequest, "store_required", "staff must be assigned to a store")
		return
	}
	if in.StoreID != nil {
		store, err := h.q.GetStoreByID(c.Request.Context(), *in.StoreID)
		if err != nil || store.BrandID != brandID {
			httpx.Fail(c, http.StatusUnprocessableEntity, "invalid_store", "store does not belong to this brand")
			return
		}
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	row, err := h.q.CreateUser(c.Request.Context(), db.CreateUserParams{
		BrandID: &brandID, StoreID: in.StoreID, RoleCode: in.Role,
		Email: &email, Phone: in.Phone, FullName: in.FullName, PasswordHash: &hash,
	})
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusCreated, auth.UserView{
		ID: row.ID, BrandID: row.BrandID, StoreID: row.StoreID, Role: row.RoleCode,
		Level: roleLevels[row.RoleCode], Email: row.Email, FullName: row.FullName,
	})
}
