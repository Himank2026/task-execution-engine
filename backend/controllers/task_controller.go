package controllers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Himank2026/task-execution-engine/backend/ratelimit"
	"github.com/Himank2026/task-execution-engine/backend/services"
)

// TaskController is the HTTP layer for tasks. It knows about requests, JSON, and
// status codes — and delegates all real work to the injected TaskService.
type TaskController struct {
	tasks   *services.TaskService
	limiter *ratelimit.Limiter
}

func NewTaskController(tasks *services.TaskService, limiter *ratelimit.Limiter) *TaskController {
	return &TaskController{tasks: tasks, limiter: limiter}
}

// rateLimited consumes n tokens of the client's task-submission budget (n = number of
// tasks). If it would exceed the limit it writes a 429 and returns true (the caller
// should stop). Fails OPEN on a Redis error so a limiter outage can't block submits.
// Only the submit endpoints use this — dashboard GETs and demo seeding are not limited.
func (tc *TaskController) rateLimited(c *gin.Context, clientID string, n int) bool {
	allowed, remaining, err := tc.limiter.AllowN(c.Request.Context(), clientID, n)
	if err != nil {
		return false
	}
	if !allowed {
		c.Header("X-RateLimit-Limit", strconv.Itoa(tc.limiter.Max()))
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": fmt.Sprintf("rate limit: max %d tasks per minute — submit fewer or wait", tc.limiter.Max()),
		})
		return true
	}
	c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
	return false
}

// Create handles POST /api/tasks. It binds+validates the JSON body, reads the
// authenticated client (set by the auth middleware), and asks the service to
// create the task.
func (tc *TaskController) Create(c *gin.Context) {
	var in services.CreateTaskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		// 400: the client sent bad/missing fields (validation failed).
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set by APIKeyAuth middleware; trustworthy because the request got past auth.
	clientID := c.GetString("client_id")
	if tc.rateLimited(c, clientID, 1) {
		return
	}

	task, err := tc.tasks.CreateTask(clientID, in)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusCreated, task)
}

// batchRequest is the body for POST /api/tasks/batch: an array of task inputs. `dive`
// validates each element with its own binding rules.
type batchRequest struct {
	Tasks []services.CreateTaskInput `json:"tasks" binding:"required,min=1,max=200,dive"`
}

// CreateBatch handles POST /api/tasks/batch — submit many tasks in one request.
func (tc *TaskController) CreateBatch(c *gin.Context) {
	var in batchRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	clientID := c.GetString("client_id")
	if tc.rateLimited(c, clientID, len(in.Tasks)) {
		return
	}

	created, err := tc.tasks.BatchCreateTasks(clientID, in.Tasks)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"created": created})
}

// List handles GET /api/tasks. It reads optional query params into a filter and
// returns a paginated page of the authenticated client's tasks.
func (tc *TaskController) List(c *gin.Context) {
	clientID := c.GetString("client_id")

	f := services.ListTasksFilter{
		Status:       c.Query("status"),
		Type:         c.Query("type"),
		SortBy:       c.Query("sort_by"),
		Order:        c.Query("order"),
		FilterClient: c.Query("client"),
		// ?all=true returns tasks across ALL clients (the dashboard's ops view). The
		// default (no param) stays per-client, preserving multi-tenant isolation.
		AllClients: c.Query("all") == "true",
	}

	// priority is optional; only set it if a valid number was given.
	if p := c.Query("priority"); p != "" {
		if v, err := strconv.ParseUint(p, 10, 8); err == nil {
			pr := uint8(v)
			f.Priority = &pr
		}
	}
	// page / page_size are optional; the service applies defaults/bounds.
	if v, err := strconv.Atoi(c.Query("page")); err == nil {
		f.Page = v
	}
	if v, err := strconv.Atoi(c.Query("page_size")); err == nil {
		f.PageSize = v
	}

	result, err := tc.tasks.ListTasks(clientID, f)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetByID handles GET /api/tasks/:id. It parses the id from the URL, then asks
// the service for that task scoped to the authenticated client.
func (tc *TaskController) GetByID(c *gin.Context) {
	// Path params arrive as strings; convert to the uint64 our id column uses.
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id"})
		return
	}

	clientID := c.GetString("client_id")

	task, err := tc.tasks.GetTask(clientID, id)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, task)
}

// Cancel handles POST /api/tasks/:id/cancel. It transitions a pending/running
// task to cancelled, rejecting tasks that are already in a terminal state.
func (tc *TaskController) Cancel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id"})
		return
	}

	clientID := c.GetString("client_id")

	task, err := tc.tasks.CancelTask(clientID, id)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, task)
}

// SeedDemo handles POST /api/demo/seed. DEV/DEMO ONLY: appends a batch of random tasks
// spread across all clients. Clicking it repeatedly grows the dataset; /demo/clear
// resets it to empty.
func (tc *TaskController) SeedDemo(c *gin.Context) {
	const demoCount = 50

	if err := tc.tasks.AddDemoTasks(demoCount); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"added": demoCount})
}

// ClearDemo handles POST /api/demo/clear. DEV/DEMO ONLY: wipes ALL tasks and
// dead-letter rows so the dashboard resets to an empty slate.
func (tc *TaskController) ClearDemo(c *gin.Context) {
	if err := tc.tasks.ClearAllData(); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cleared": true})
}
