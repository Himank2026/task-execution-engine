package routes

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Himank2026/task-execution-engine/backend/controllers"
	"github.com/Himank2026/task-execution-engine/backend/middleware"
	"github.com/Himank2026/task-execution-engine/backend/services"
)

// SetupRouter builds the Gin engine and wires the dependency graph:
// db -> service -> controller, then registers routes. This is where DI happens —
// each layer is constructed once here and handed its dependencies.
func SetupRouter(db *gorm.DB) *gin.Engine {
	r := gin.Default()

	// Build the layers, inside-out.
	taskService := services.NewTaskService(db)
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
