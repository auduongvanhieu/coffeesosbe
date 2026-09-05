package auth

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"coffeesos/internal/db"
	"coffeesos/internal/httpx"
	"coffeesos/internal/tenant"
)

type Handler struct {
	q      *db.Queries
	issuer *TokenIssuer
}

func NewHandler(q *db.Queries, issuer *TokenIssuer) *Handler {
	return &Handler{q: q, issuer: issuer}
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6,max=128"`
}

type UserView struct {
	ID       uuid.UUID  `json:"id"`
	BrandID  *uuid.UUID `json:"brandId"`
	StoreID  *uuid.UUID `json:"storeId"`
	Role     string     `json:"role"`
	Level    int        `json:"level"`
	Email    *string    `json:"email"`
	FullName string     `json:"fullName"`
}

type loginResponse struct {
	AccessToken string    `json:"accessToken"`
	TokenType   string    `json:"tokenType"`
	ExpiresAt   time.Time `json:"expiresAt"`
	User        UserView  `json:"user"`
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

	p := tenant.Principal{
		UserID:  u.ID,
		BrandID: u.BrandID,
		StoreID: u.StoreID,
		Role:    u.RoleCode,
		Level:   int(u.RoleLevel),
	}
	token, exp, err := h.issuer.Issue(p)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusOK, loginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   exp,
		User: UserView{
			ID: u.ID, BrandID: u.BrandID, StoreID: u.StoreID,
			Role: u.RoleCode, Level: int(u.RoleLevel), Email: u.Email, FullName: u.FullName,
		},
	})
}

// Me handles GET /api/v1/auth/me.
func (h *Handler) Me(c *gin.Context) {
	p := tenant.MustFrom(c)
	u, err := h.q.GetUserByID(c.Request.Context(), p.UserID)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusOK, UserView{
		ID: u.ID, BrandID: u.BrandID, StoreID: u.StoreID,
		Role: u.RoleCode, Level: int(u.RoleLevel), Email: u.Email, FullName: u.FullName,
	})
}
