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
	allClients := c.Query("all") == "true"

	summary, err := ac.analytics.GetSummary(clientID, allClients)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, summary)
}

// Throughput handles GET /api/analytics/throughput — a time-series of completions over
// the last few minutes, for the throughput chart.
func (ac *AnalyticsController) Throughput(c *gin.Context) {
	clientID := c.GetString("client_id")
	allClients := c.Query("all") == "true"

	points, err := ac.analytics.GetThroughput(clientID, allClients)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, points)
}

// Types handles GET /api/analytics/types — per-task-type breakdown (status %, timings).
func (ac *AnalyticsController) Types(c *gin.Context) {
	clientID := c.GetString("client_id")
	allClients := c.Query("all") == "true"

	stats, err := ac.analytics.GetTypeBreakdown(clientID, allClients)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, stats)
}
