package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"coffeesos/internal/httpx"
	"coffeesos/internal/tenant"
)

// RequireAuth validates the bearer token and stores the principal in context.
// A `token` query parameter is also accepted so browsers can open WebSockets.
func RequireAuth(issuer *TokenIssuer) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := bearerToken(c)
		if raw == "" {
			httpx.Fail(c, http.StatusUnauthorized, "unauthorized", "missing bearer token")
			return
		}
		p, err := issuer.Parse(raw)
		if err != nil {
			httpx.Fail(c, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
			return
		}
		tenant.Set(c, p)
		c.Next()
	}
}

// RequireLevel rejects callers whose role level is below min.
func RequireLevel(min int) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := tenant.From(c)
		if !ok {
			httpx.Fail(c, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		if p.Level < min {
			httpx.Fail(c, http.StatusForbidden, "forbidden", "insufficient role")
			return
		}
		c.Next()
	}
}

func bearerToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); h != "" {
		if parts := strings.SplitN(h, " ", 2); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}
	return strings.TrimSpace(c.Query("token"))
}
