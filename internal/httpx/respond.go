// Package httpx holds small helpers shared by HTTP handlers.
package httpx

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Fail writes a JSON error and aborts the handler chain.
func Fail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": ErrorBody{Code: code, Message: message}})
}

// FailDB maps common database errors to HTTP responses.
func FailDB(c *gin.Context, err error) {
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		Fail(c, http.StatusNotFound, "not_found", "resource not found")
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		Fail(c, http.StatusConflict, "conflict", "resource already exists")
	case errors.As(err, &pgErr) && pgErr.Code == "23503":
		Fail(c, http.StatusUnprocessableEntity, "invalid_reference", "referenced resource does not exist in this brand")
	default:
		_ = c.Error(err)
		Fail(c, http.StatusInternalServerError, "internal", "internal server error")
	}
}

// Bind parses JSON into dst and writes a 400 on failure. The validator's
// message ("Key: 'loginRequest.Email' Error:Field validation for ...") is
// developer noise, so it goes to the request log and the client gets a short
// Vietnamese sentence naming the field.
func Bind(c *gin.Context, dst any) bool {
	err := c.ShouldBindJSON(dst)
	if err == nil {
		return true
	}
	_ = c.Error(err)
	Fail(c, http.StatusBadRequest, "invalid_request", bindMessage(err))
	return false
}

// fieldLabels maps struct field names to what the clients call them.
var fieldLabels = map[string]string{
	"Email": "email", "Password": "mật khẩu", "PIN": "mã PIN", "Phone": "số điện thoại",
	"Name": "tên", "StoreID": "cửa hàng", "ItemID": "món", "Quantity": "số lượng",
	"Method": "phương thức thanh toán", "Status": "trạng thái", "OrderType": "loại đơn",
	"Items": "danh sách món", "CustomerName": "tên khách", "CustomerPhone": "số điện thoại khách",
	"AvatarURL": "ảnh đại diện", "PromotionCode": "mã giảm giá",
}

func bindMessage(err error) string {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) || len(ve) == 0 {
		return "dữ liệu gửi lên không hợp lệ"
	}
	fe := ve[0]
	label, ok := fieldLabels[fe.Field()]
	if !ok {
		label = strings.ToLower(fe.Field())
	}
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("thiếu %s", label)
	case "email":
		return "email không hợp lệ"
	case "min":
		return fmt.Sprintf("%s phải có ít nhất %s ký tự", label, fe.Param())
	case "max":
		return fmt.Sprintf("%s quá dài (tối đa %s)", label, fe.Param())
	case "oneof":
		return fmt.Sprintf("%s không hợp lệ", label)
	case "url":
		return fmt.Sprintf("%s phải là một đường dẫn hợp lệ", label)
	default:
		return fmt.Sprintf("%s không hợp lệ", label)
	}
}
