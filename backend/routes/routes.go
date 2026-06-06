package routes

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Himank2026/task-execution-engine/backend/controllers"
	"github.com/Himank2026/task-execution-engine/backend/middleware"
	"github.com/Himank2026/task-execution-engine/backend/services"
)

// SetupRouter builds the Gin engine and wires the dependency graph:
// service -> controller, then registers routes. The TaskService is passed in
// (not built here) so main can share ONE instance between the HTTP layer and the
// worker pool — both must operate on the same business logic. db is still needed
// for the auth middleware's key lookups.
func SetupRouter(db *gorm.DB, taskService *services.TaskService) *gin.Engine {
	r := gin.Default()

	// Build the controller on top of the shared service.
	taskController := controllers.NewTaskController(taskService)

	api := r.Group("/api")

	// Public: health checks must not require auth (load balancers probe them).
	api.GET("/health", controllers.HealthCheck)

	// Protected: everything here runs APIKeyAuth first, so handlers can trust
	// that a valid client_id is in the context.
	authed := api.Group("")
	authed.Use(middleware.APIKeyAuth(db))
	{
		authed.POST("/tasks", taskController.Create)
		authed.GET("/tasks", taskController.List)
		authed.GET("/tasks/:id", taskController.GetByID)
		authed.POST("/tasks/:id/cancel", taskController.Cancel)
	}

	return r
}
