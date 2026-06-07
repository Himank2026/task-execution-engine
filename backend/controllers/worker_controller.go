package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Himank2026/task-execution-engine/backend/worker"
)

// WorkerController exposes the live state of the worker pool across ALL backend
// instances (an ops view). It reads the shared registry in Redis, so it returns every
// instance's workers no matter which instance handled the request.
type WorkerController struct {
	registry *worker.Registry
}

func NewWorkerController(registry *worker.Registry) *WorkerController {
	return &WorkerController{registry: registry}
}

// List handles GET /api/workers — every instance and what each of its workers is doing.
func (wc *WorkerController) List(c *gin.Context) {
	instances, err := wc.registry.AllInstances(c.Request.Context())
	if err != nil {
		// Don't fail the dashboard on a Redis hiccup; return an empty set.
		c.JSON(http.StatusOK, gin.H{"instances": []worker.InstanceWorkers{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"instances": instances})
}
