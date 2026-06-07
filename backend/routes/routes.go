package routes

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Himank2026/task-execution-engine/backend/controllers"
	"github.com/Himank2026/task-execution-engine/backend/middleware"
	"github.com/Himank2026/task-execution-engine/backend/ratelimit"
	"github.com/Himank2026/task-execution-engine/backend/services"
	"github.com/Himank2026/task-execution-engine/backend/sse"
)

// SetupRouter builds the Gin engine and wires the dependency graph:
// service -> controller, then registers routes. The TaskService is passed in
// (not built here) so main can share ONE instance between the HTTP layer and the
// worker pool — both must operate on the same business logic. db is still needed
// for the auth middleware's key lookups.
func SetupRouter(db *gorm.DB, taskService *services.TaskService, limiter *ratelimit.Limiter, hub *sse.Hub) *gin.Engine {
	r := gin.Default()

	// Build the controllers on top of the services. The task service is shared (passed
	// in); the analytics service is read-only, so we build it here from db.
	taskController := controllers.NewTaskController(taskService)
	analyticsController := controllers.NewAnalyticsController(services.NewAnalyticsService(db))
	sseController := controllers.NewSSEController(hub)

	api := r.Group("/api")

	// Public: health checks must not require auth (load balancers probe them).
	api.GET("/health", controllers.HealthCheck)

	// Protected: everything here runs APIKeyAuth FIRST (so a valid client_id is in the
	// context), THEN the per-client rate limiter (which reads that client_id). Order
	// matters: auth must populate the client before the limiter counts against it.
	authed := api.Group("")
	authed.Use(middleware.APIKeyAuth(db))
	authed.Use(middleware.RateLimit(limiter))
	{
		authed.POST("/tasks", taskController.Create)
		authed.GET("/tasks", taskController.List)
		authed.GET("/tasks/:id", taskController.GetByID)
		authed.POST("/tasks/:id/cancel", taskController.Cancel)

		authed.GET("/analytics", analyticsController.Summary)

		authed.GET("/sse/events", sseController.Stream)
	}

	return r
}
