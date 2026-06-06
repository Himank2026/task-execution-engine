package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/Himank2026/task-execution-engine/backend/controllers"
)

// SetupRouter builds the Gin engine and registers every route under /api.
// As features are added, give each its own register function (e.g. registerTaskRoutes).
func SetupRouter() *gin.Engine {
	r := gin.Default()

	api := r.Group("/api")
	api.GET("/health", controllers.HealthCheck)

	return r
}
