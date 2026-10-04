package auth

import (
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"coffeesos/internal/db"
	"coffeesos/internal/httpx"
	"coffeesos/internal/storage"
	"coffeesos/internal/tenant"
)

type Handler struct {
	q       *db.Queries
	issuer  *TokenIssuer
	uploads *storage.Handler // nil disables avatar uploads
}

func NewHandler(q *db.Queries, issuer *TokenIssuer, uploads *storage.Handler) *Handler {
	return &Handler{q: q, issuer: issuer, uploads: uploads}
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6,max=128"`
}

type pinLoginRequest struct {
	StoreID uuid.UUID `json:"storeId" binding:"required"`
	PIN     string    `json:"pin" binding:"required,min=4,max=8"`
}

// UserView is the identity payload returned by login and /auth/me. The scope
// names let the POS render "Daily Bean · Quận 3" without extra calls.
type UserView struct {
	ID           uuid.UUID  `json:"id"`
	BrandID      *uuid.UUID `json:"brandId"`
	StoreID      *uuid.UUID `json:"storeId"`
	Role         string     `json:"role"`
	Level        int        `json:"level"`
	Email        *string    `json:"email"`
	FullName     string     `json:"fullName"`
	AvatarURL    *string    `json:"avatarUrl"`
	BrandName    *string    `json:"brandName"`
	BrandLogoURL *string    `json:"brandLogoUrl"`
	StoreName    *string    `json:"storeName"`
}

type loginResponse struct {
	AccessToken string    `json:"accessToken"`
	TokenType   string    `json:"tokenType"`
	ExpiresAt   time.Time `json:"expiresAt"`
	User        UserView  `json:"user"`
}

// userRow is the common subset of the sqlc rows that carry role_level.
type userRow struct {
	ID       uuid.UUID
	BrandID  *uuid.UUID
	StoreID  *uuid.UUID
	RoleCode string
	Level    int
	Email    *string
	FullName string
	Avatar   *string
}

// Login handles POST /api/v1/auth/login for staff, managers, owners and
// platform admins. Customers (phone + OTP) will get their own flow later.
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if !httpx.Bind(c, &req) {
		return
	}
	u, err := h.q.GetUserByEmail(c.Request.Context(), req.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(c, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
			return
		}
		httpx.FailDB(c, err)
		return
	}
	if !u.IsActive || u.PasswordHash == nil || !CheckPassword(*u.PasswordHash, req.Password) {
		httpx.Fail(c, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
		return
	}
	h.respondLogin(c, userRow{
		ID: u.ID, BrandID: u.BrandID, StoreID: u.StoreID, RoleCode: u.RoleCode, Level: int(u.RoleLevel), Email: u.Email, FullName: u.FullName, Avatar: u.AvatarUrl,
	})
}

var pinRe = regexp.MustCompile(`^[0-9]{4,8}$`)

// PinLogin handles POST /api/v1/auth/pin-login for store terminals: the device
// already knows its store; the staff member types a short numeric PIN.
func (h *Handler) PinLogin(c *gin.Context) {
	var req pinLoginRequest
	if !httpx.Bind(c, &req) {
		return
	}
	if !pinRe.MatchString(req.PIN) {
		httpx.Fail(c, http.StatusUnauthorized, "invalid_pin", "mã PIN không đúng")
		return
	}
	candidates, err := h.q.ListPinUsersByStore(c.Request.Context(), req.StoreID)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	for _, u := range candidates {
		if u.PinHash != nil && CheckPassword(*u.PinHash, req.PIN) {
			storeID := u.StoreID
			if storeID == nil {
				// Brand-level manager logging into a terminal: pin the session to this store.
				sid := req.StoreID
				storeID = &sid
			}
			h.respondLogin(c, userRow{
				ID: u.ID, BrandID: u.BrandID, StoreID: storeID, RoleCode: u.RoleCode, Level: int(u.RoleLevel), Email: u.Email, FullName: u.FullName, Avatar: u.AvatarUrl,
			})
			return
		}
	}
	httpx.Fail(c, http.StatusUnauthorized, "invalid_pin", "mã PIN không đúng")
}

func (h *Handler) respondLogin(c *gin.Context, u userRow) {
	p := tenant.Principal{UserID: u.ID, BrandID: u.BrandID, StoreID: u.StoreID, Role: u.RoleCode, Level: u.Level}
	token, exp, err := h.issuer.Issue(p)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	view := h.view(c, u)
	c.JSON(http.StatusOK, loginResponse{AccessToken: token, TokenType: "Bearer", ExpiresAt: exp, User: view})
}

// Me handles GET /api/v1/auth/me.
func (h *Handler) Me(c *gin.Context) {
	p := tenant.MustFrom(c)
	u, err := h.q.GetUserByID(c.Request.Context(), p.UserID)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	storeID := u.StoreID
	if storeID == nil && p.StoreID != nil {
		storeID = p.StoreID // manager session pinned to a terminal's store
	}
	c.JSON(http.StatusOK, h.view(c, userRow{
		ID: u.ID, BrandID: u.BrandID, StoreID: storeID, RoleCode: u.RoleCode, Level: int(u.RoleLevel), Email: u.Email, FullName: u.FullName, Avatar: u.AvatarUrl,
	}))
}

func (h *Handler) view(c *gin.Context, u userRow) UserView {
	v := UserView{ID: u.ID, BrandID: u.BrandID, StoreID: u.StoreID, Role: u.RoleCode, Level: u.Level, Email: u.Email, FullName: u.FullName, AvatarURL: u.Avatar}
	if scope, err := h.q.GetUserScope(c.Request.Context(), u.ID); err == nil {
		v.BrandName, v.BrandLogoURL, v.StoreName = scope.BrandName, scope.BrandLogoUrl, scope.StoreName
	}
	if v.StoreName == nil && u.StoreID != nil {
		if st, err := h.q.GetStoreByID(c.Request.Context(), *u.StoreID); err == nil {
			v.StoreName = &st.Name
		}
	}
	return v
}

type setPinRequest struct {
	PIN string `json:"pin" binding:"required,min=4,max=8"`
}

// SetPin handles PUT /api/v1/admin/users/:id/pin (store manager and above).
func (h *Handler) SetPin(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_id", "user id must be a UUID")
		return
	}
	var req setPinRequest
	if !httpx.Bind(c, &req) {
		return
	}
	if !pinRe.MatchString(req.PIN) {
		httpx.Fail(c, http.StatusBadRequest, "invalid_pin", "PIN must be 4-8 digits")
		return
	}
	hash, err := HashPassword(req.PIN)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	if err := h.q.SetUserPin(c.Request.Context(), db.SetUserPinParams{ID: id, BrandID: &brandID, PinHash: &hash}); err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// UploadAvatar handles POST /api/v1/pos/me/avatar (multipart "file"): the
// signed-in staff member changes their own profile photo.
func (h *Handler) UploadAvatar(c *gin.Context) {
	p := tenant.MustFrom(c)
	if h.uploads == nil || p.BrandID == nil {
		httpx.Fail(c, http.StatusServiceUnavailable, "storage_not_configured", "file storage is not configured on this server")
		return
	}
	obj, ok := h.uploads.Receive(c, *p.BrandID, storage.KindAvatar)
	if !ok {
		return
	}
	u, err := h.q.SetUserAvatar(c.Request.Context(), db.SetUserAvatarParams{ID: p.UserID, AvatarUrl: &obj.URL})
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusOK, h.view(c, userRow{
		ID: u.ID, BrandID: u.BrandID, StoreID: p.StoreID, RoleCode: u.RoleCode, Level: p.Level, Email: u.Email, FullName: u.FullName, Avatar: u.AvatarUrl,
	}))
}

type setAvatarRequest struct {
	AvatarURL *string `json:"avatarUrl" binding:"omitempty,url"`
}

// SetAvatar handles PUT /api/v1/admin/users/:id/avatar {avatarUrl} (store
// manager and above); null clears the photo.
func (h *Handler) SetAvatar(c *gin.Context) {
	brandID, ok := tenant.BrandFrom(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_id", "user id must be a UUID")
		return
	}
	var req setAvatarRequest
	if !httpx.Bind(c, &req) {
		return
	}
	u, err := h.q.SetUserAvatarInBrand(c.Request.Context(), db.SetUserAvatarInBrandParams{ID: id, BrandID: &brandID, AvatarUrl: req.AvatarURL})
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": u.ID, "avatarUrl": u.AvatarUrl})
}
