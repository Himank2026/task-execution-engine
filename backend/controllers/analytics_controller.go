package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Himank2026/task-execution-engine/backend/services"
)

// AnalyticsController exposes the read-only reporting endpoints.
type AnalyticsController struct {
	analytics *services.AnalyticsService
}

func NewAnalyticsController(analytics *services.AnalyticsService) *AnalyticsController {
	return &AnalyticsController{analytics: analytics}
}

// Summary handles GET /api/analytics — per-client task metrics for the dashboard.
// client_id comes from the auth middleware, so the numbers are scoped to the caller.
func (ac *AnalyticsController) Summary(c *gin.Context) {
	clientID := c.GetString("client_id")

	summary, err := ac.analytics.GetSummary(clientID)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, summary)
}
