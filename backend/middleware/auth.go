package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Himank2026/task-execution-engine/backend/models"
)

// APIKeyAuth returns Gin middleware that gates protected routes. It reads the
// x-api-key header, looks it up in the api_keys table, and either:
//   - rejects the request (401) if the key is missing, unknown, or inactive, or
//   - stashes the owning client_id in the request context and calls c.Next() so
//     the real handler can run.
//
// Downstream handlers read the client with c.GetString("client_id") — they never
// see the raw key. (Optimization for later: cache key->client in Redis so we
// don't hit MySQL on every request. Correctness first, caching next.)
func APIKeyAuth(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("x-api-key")
		if key == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing x-api-key header"})
			return
		}

		var apiKey models.APIKey
		if err := db.Where("api_key = ? AND active = ?", key, true).First(&apiKey).Error; err != nil {
			// Either no row matched or some DB error — in both cases we refuse to
			// authenticate. We don't leak which, to avoid helping key-guessing.
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or inactive api key"})
			return
		}

		c.Set("client_id", apiKey.ClientID)
		c.Next()
	}
}
