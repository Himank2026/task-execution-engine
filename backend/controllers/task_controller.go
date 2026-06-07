package controllers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Himank2026/task-execution-engine/backend/services"
)

// TaskController is the HTTP layer for tasks. It knows about requests, JSON, and
// status codes — and delegates all real work to the injected TaskService.
type TaskController struct {
	tasks *services.TaskService
}

func NewTaskController(tasks *services.TaskService) *TaskController {
	return &TaskController{tasks: tasks}
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

	task, err := tc.tasks.CreateTask(clientID, in)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusCreated, task)
}

// List handles GET /api/tasks. It reads optional query params into a filter and
// returns a paginated page of the authenticated client's tasks.
func (tc *TaskController) List(c *gin.Context) {
	clientID := c.GetString("client_id")

	f := services.ListTasksFilter{
		Status: c.Query("status"),
		Type:   c.Query("type"),
		SortBy: c.Query("sort_by"),
		Order:  c.Query("order"),
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

// ResetDemo handles POST /api/demo/reset. DEV/DEMO ONLY: it wipes the calling client's
// tasks and seeds a fresh batch of random ones, so demos start clean and the table
// doesn't accumulate test rows. Scoped to the caller — never touches other tenants.
func (tc *TaskController) ResetDemo(c *gin.Context) {
	clientID := c.GetString("client_id")
	const demoCount = 50

	if err := tc.tasks.ResetDemoData(clientID, demoCount); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"created": demoCount})
}
