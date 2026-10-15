package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"coffeesos/internal/db"
	"coffeesos/internal/httpx"
	"coffeesos/internal/storage"
	"coffeesos/internal/tenant"
)

// How long a password-reset link stays valid.
const resetTTL = time.Hour

type updateProfileRequest struct {
	FullName string  `json:"fullName" binding:"required,min=2,max=100"`
	Email    string  `json:"email" binding:"required,email"`
	Phone    *string `json:"phone" binding:"omitempty,max=20"`
}

// UpdateProfile handles PUT /api/v1/auth/me: the signed-in user edits their
// own name, email and phone.
func (h *Handler) UpdateProfile(c *gin.Context) {
	p := tenant.MustFrom(c)
	var req updateProfileRequest
	if !httpx.Bind(c, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	phone := req.Phone
	if phone != nil && strings.TrimSpace(*phone) == "" {
		phone = nil
	}
	u, err := h.q.UpdateUserProfile(c.Request.Context(), db.UpdateUserProfileParams{
		ID: p.UserID, FullName: strings.TrimSpace(req.FullName), Email: &email, Phone: phone,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			httpx.Fail(c, http.StatusConflict, "email_taken", "email này đã được dùng cho tài khoản khác")
			return
		}
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusOK, h.view(c, userRow{
		ID: u.ID, BrandID: u.BrandID, StoreID: u.StoreID, RoleCode: u.RoleCode, Level: p.Level,
		Email: u.Email, FullName: u.FullName, Avatar: u.AvatarUrl,
	}))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" binding:"required"`
	NewPassword     string `json:"newPassword" binding:"required,min=8,max=128"`
}

// ChangePassword handles POST /api/v1/auth/change-password.
func (h *Handler) ChangePassword(c *gin.Context) {
	p := tenant.MustFrom(c)
	var req changePasswordRequest
	if !httpx.Bind(c, &req) {
		return
	}
	u, err := h.q.GetUserByID(c.Request.Context(), p.UserID)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	if u.PasswordHash == nil || !CheckPassword(*u.PasswordHash, req.CurrentPassword) {
		httpx.Fail(c, http.StatusUnauthorized, "wrong_password", "mật khẩu hiện tại không đúng")
		return
	}
	if req.NewPassword == req.CurrentPassword {
		httpx.Fail(c, http.StatusUnprocessableEntity, "same_password", "mật khẩu mới phải khác mật khẩu cũ")
		return
	}
	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	if err := h.q.SetUserPassword(c.Request.Context(), db.SetUserPasswordParams{ID: p.UserID, PasswordHash: &hash}); err != nil {
		httpx.FailDB(c, err)
		return
	}
	// Any outstanding reset link is now stale.
	_ = h.q.InvalidateUserResets(c.Request.Context(), p.UserID)
	c.Status(http.StatusNoContent)
}

type forgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ForgotPassword handles POST /api/v1/auth/forgot-password. It always answers
// 204 so the endpoint cannot be used to discover which emails have accounts.
func (h *Handler) ForgotPassword(c *gin.Context) {
	var req forgotPasswordRequest
	if !httpx.Bind(c, &req) {
		return
	}
	ctx := c.Request.Context()
	defer c.Status(http.StatusNoContent)

	u, err := h.q.GetUserByEmail(ctx, strings.TrimSpace(req.Email))
	if err != nil || !u.IsActive {
		return // unknown address: say nothing
	}
	token, hash, err := newResetToken()
	if err != nil {
		_ = c.Error(err)
		return
	}
	_ = h.q.InvalidateUserResets(ctx, u.ID)
	if _, err := h.q.CreatePasswordReset(ctx, db.CreatePasswordResetParams{
		UserID: u.ID, TokenHash: hash, ExpiresAt: time.Now().Add(resetTTL),
	}); err != nil {
		_ = c.Error(err)
		return
	}
	link := fmt.Sprintf("%s/reset-password?token=%s", strings.TrimRight(h.adminURL, "/"), token)
	body := strings.Join([]string{
		"Xin chào " + u.FullName + ",",
		"",
		"Bạn vừa yêu cầu đặt lại mật khẩu CoffeeSOS. Mở liên kết dưới đây để đặt mật khẩu mới:",
		link,
		"",
		fmt.Sprintf("Liên kết có hiệu lực trong %d phút và chỉ dùng được một lần.", int(resetTTL.Minutes())),
		"Nếu bạn không yêu cầu, hãy bỏ qua email này.",
	}, "\n")
	if err := h.mailer.Send(ctx, strings.TrimSpace(req.Email), "Đặt lại mật khẩu CoffeeSOS", body); err != nil {
		_ = c.Error(err) // logged; the caller still gets 204
	}
}

type resetPasswordRequest struct {
	Token       string `json:"token" binding:"required,min=10"`
	NewPassword string `json:"newPassword" binding:"required,min=8,max=128"`
}

// ResetPassword handles POST /api/v1/auth/reset-password with the token from
// the email.
func (h *Handler) ResetPassword(c *gin.Context) {
	var req resetPasswordRequest
	if !httpx.Bind(c, &req) {
		return
	}
	ctx := c.Request.Context()
	row, err := h.q.GetPasswordReset(ctx, hashToken(req.Token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(c, http.StatusBadRequest, "invalid_token", "liên kết không hợp lệ hoặc đã được dùng")
			return
		}
		httpx.FailDB(c, err)
		return
	}
	if row.UsedAt != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_token", "liên kết này đã được dùng, vui lòng yêu cầu lại")
		return
	}
	if time.Now().After(row.ExpiresAt) {
		httpx.Fail(c, http.StatusBadRequest, "invalid_token", "liên kết đã hết hạn, vui lòng yêu cầu lại")
		return
	}
	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	if err := h.q.SetUserPassword(ctx, db.SetUserPasswordParams{ID: row.UserID, PasswordHash: &hash}); err != nil {
		httpx.FailDB(c, err)
		return
	}
	if err := h.q.UsePasswordReset(ctx, row.ID); err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// UploadMyAvatar handles POST /api/v1/auth/me/avatar for any signed-in user,
// including platform admins who belong to no brand.
func (h *Handler) UploadMyAvatar(c *gin.Context) {
	p := tenant.MustFrom(c)
	if h.uploads == nil {
		httpx.Fail(c, http.StatusServiceUnavailable, "storage_not_configured", "file storage is not configured on this server")
		return
	}
	brandID := uuidOrNil(p.BrandID)
	obj, ok := h.uploads.Receive(c, brandID, storage.KindAvatar)
	if !ok {
		return
	}
	u, err := h.q.SetUserAvatar(c.Request.Context(), db.SetUserAvatarParams{ID: p.UserID, AvatarUrl: &obj.URL})
	if err != nil {
		httpx.FailDB(c, err)
		return
	}
	c.JSON(http.StatusOK, h.view(c, userRow{
		ID: u.ID, BrandID: u.BrandID, StoreID: p.StoreID, RoleCode: u.RoleCode, Level: p.Level,
		Email: u.Email, FullName: u.FullName, Avatar: u.AvatarUrl,
	}))
}

// newResetToken returns the token to mail and the hash to store.
func newResetToken() (token, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// uuidOrNil lets platform admins (no brand) upload too; storage files them
// under a platform folder.
func uuidOrNil(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}
