package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthCheck reports that the server is up. Returns 200 with a small JSON body.
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}
