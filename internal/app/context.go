package app

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

func contextWithTimeout(c *gin.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), d)
}
