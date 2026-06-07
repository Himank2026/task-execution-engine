package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Himank2026/task-execution-engine/backend/worker"
)

// WorkerController exposes the live state of the worker pool (an ops-style view: it
// reflects GLOBAL worker activity on this instance, across all clients).
type WorkerController struct {
	pool *worker.Pool
}

func NewWorkerController(pool *worker.Pool) *WorkerController {
	return &WorkerController{pool: pool}
}

// List handles GET /api/workers — which task each worker is running, or idle.
func (wc *WorkerController) List(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"instance": wc.pool.InstanceID(),
		"workers":  wc.pool.WorkerStates(),
	})
}
