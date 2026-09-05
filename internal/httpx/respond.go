// Package httpx holds small helpers shared by HTTP handlers.
package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
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

// Bind parses JSON into dst and writes a 400 on failure.
func Bind(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return false
	}
	return true
}
