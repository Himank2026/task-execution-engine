package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Himank2026/task-execution-engine/backend/ratelimit"
)

// RateLimit returns Gin middleware that enforces a per-client sliding-window limit.
//
// IMPORTANT: it must be registered AFTER APIKeyAuth, because it identifies the caller
// by the client_id that auth puts in the context. We rate-limit per client (not per
// IP) so one tenant's burst can't eat another tenant's allowance.
func RateLimit(limiter *ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientID := c.GetString("client_id")
		if clientID == "" {
			// Auth should have set this; if somehow not, don't hard-fail the request.
			c.Next()
			return
		}

		allowed, remaining, err := limiter.Allow(c.Request.Context(), clientID)
		if err != nil {
			// The client simply went away mid-request (e.g. closed an SSE stream) — the
			// context is cancelled and there's nothing left to serve. Not an error.
			if errors.Is(err, context.Canceled) {
				return
			}
			// Redis is down or erroring. We FAIL OPEN: a limiter outage shouldn't take
			// the whole API down with it. (Phase 6 follow-up: an in-memory fallback
			// limiter instead of letting everything through.)
			slog.Error("rate limiter error, allowing request", "client", clientID, "err", err)
			c.Next()
			return
		}

		// Expose the policy to clients via standard-ish headers.
		c.Header("X-RateLimit-Limit", strconv.Itoa(limiter.Max()))

		if !allowed {
			c.Header("X-RateLimit-Remaining", "0")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded, slow down",
			})
			return
		}

		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Next()
	}
}
