package controllers

import (
	"io"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Himank2026/task-execution-engine/backend/sse"
)

// SSEController serves the live event stream.
type SSEController struct {
	hub *sse.Hub
}

func NewSSEController(hub *sse.Hub) *SSEController {
	return &SSEController{hub: hub}
}

// Stream handles GET /api/sse/events. It holds the HTTP connection open and pushes
// this client's task events as Server-Sent Events until the client disconnects.
func (sc *SSEController) Stream(c *gin.Context) {
	clientID := c.GetString("client_id")

	// Register this connection; always clean up on the way out.
	sub := sc.hub.Subscribe(clientID)
	defer sc.hub.Unsubscribe(sub)

	// The headers that make a response an SSE stream.
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// Heartbeat: send a comment-ish event periodically so the connection stays alive
	// through proxies, and so we notice a dead client even when no tasks are running.
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// c.Stream loops as long as the callback returns true, flushing after each write.
	c.Stream(func(w io.Writer) bool {
		select {
		case e, ok := <-sub.Events():
			if !ok {
				return false // hub closed our channel
			}
			c.SSEvent(e.Type, e) // writes: event: <type>\n data: <json>\n\n
			return true
		case <-ticker.C:
			c.SSEvent("heartbeat", gin.H{"time": time.Now().Unix()})
			return true
		case <-c.Request.Context().Done():
			return false // client disconnected
		}
	})
}
