package controllers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Himank2026/task-execution-engine/backend/services"
)

// respondError is the single place that maps an error returned by a service into
// an HTTP response. Centralizing it means every handler reports the same kind of
// error the same way, and adding a new mapping is a one-line change here instead
// of edits scattered across controllers.
//
// (Input-validation errors — bad JSON, bad id — are handled inline in each
// handler with a 400, because those are detected in the HTTP layer before any
// service is called.)
func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
	case errors.Is(err, services.ErrTaskNotCancellable):
		c.JSON(http.StatusConflict, gin.H{"error": "task cannot be cancelled in its current state"})
	default:
		// Unexpected error: log the detail server-side, return a generic message
		// so we never leak internals (SQL errors, stack info) to the client.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
