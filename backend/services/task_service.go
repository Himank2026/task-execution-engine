package services

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Himank2026/task-execution-engine/backend/models"
)

// ErrTaskNotCancellable is returned when a cancel is attempted on a task that has
// already reached a terminal state (completed/failed/cancelled). The controller
// maps this to HTTP 409 Conflict.
var ErrTaskNotCancellable = errors.New("task is not in a cancellable state")

// TaskService holds the business logic for tasks. It depends only on the DB —
// not on Gin or HTTP — so anything (an HTTP controller now, the worker pool
// later) can call it. The db is injected via NewTaskService.
type TaskService struct {
	db *gorm.DB
}

func NewTaskService(db *gorm.DB) *TaskService {
	return &TaskService{db: db}
}

// CreateTaskInput is the data needed to submit a task. Note what's NOT here:
// client_id (it comes from the authenticated key, not the caller) and status
// (always starts "pending"). binding tags drive validation when the controller
// parses the JSON body.
type CreateTaskInput struct {
	Type       string          `json:"type" binding:"required"`
	Priority   uint8           `json:"priority" binding:"required,min=1,max=5"`
	Payload    json.RawMessage `json:"payload"`
	MaxRetries *int            `json:"max_retries" binding:"omitempty,min=0"`
}

// CreateTask inserts a new pending task owned by clientID and returns it with
// its DB-assigned id and timestamps filled in.
func (s *TaskService) CreateTask(clientID string, in CreateTaskInput) (*models.Task, error) {
	task := &models.Task{
		ClientID:   clientID,
		Type:       in.Type,
		Priority:   in.Priority,
		Payload:    in.Payload,
		Status:     "pending", // every task enters the queue waiting to run
		MaxRetries: 3,         // sensible default; overridden below if provided
	}
	if in.MaxRetries != nil {
		task.MaxRetries = *in.MaxRetries
	}

	if err := s.db.Create(task).Error; err != nil {
		return nil, err
	}
	return task, nil
}

// allowedSortColumns is the whitelist for the `sort_by` query param. A sort
// column can't be a parameterized `?` placeholder — it becomes part of the SQL
// text — so we ONLY ever allow these exact strings. Anything else falls back to
// the default. This is the SQL-injection defense for sorting.
var allowedSortColumns = map[string]bool{
	"created_at": true,
	"priority":   true,
	"status":     true,
	"id":         true,
}

// ListTasksFilter carries the optional query options for listing tasks. Empty
// string / nil means "don't filter on this".
type ListTasksFilter struct {
	Status   string
	Type     string
	Priority *uint8
	Page     int
	PageSize int
	SortBy   string
	Order    string
}

// ListTasksResult is the page of tasks plus the metadata a UI needs to paginate.
type ListTasksResult struct {
	Tasks      []models.Task `json:"data"`
	Total      int64         `json:"total"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	TotalPages int           `json:"total_pages"`
}

// ListTasks returns a filtered, sorted, paginated page of the client's tasks.
// Everything is scoped to clientID (multi-tenant isolation), filter values use
// `?` placeholders (injection-safe), and the sort column is whitelisted.
func (s *TaskService) ListTasks(clientID string, f ListTasksFilter) (*ListTasksResult, error) {
	// Base query: always scoped to this client.
	q := s.db.Model(&models.Task{}).Where("client_id = ?", clientID)

	// Add a WHERE only for filters that were actually provided.
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	if f.Priority != nil {
		q = q.Where("priority = ?", *f.Priority)
	}

	// Count the full matching set BEFORE applying LIMIT/OFFSET, so the UI knows
	// how many pages exist.
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}

	// Sanitize sort: whitelist the column, force order to asc/desc.
	sortBy := "created_at" // default
	if allowedSortColumns[f.SortBy] {
		sortBy = f.SortBy
	}
	order := "desc" // default: newest first
	if strings.ToLower(f.Order) == "asc" {
		order = "asc"
	}

	// Normalize pagination with sane bounds.
	page := f.Page
	if page < 1 {
		page = 1
	}
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20 // default page size
	}
	if pageSize > 100 {
		pageSize = 100 // cap to protect the DB from huge pages
	}
	offset := (page - 1) * pageSize

	var tasks []models.Task
	if err := q.Order(sortBy + " " + order).Limit(pageSize).Offset(offset).Find(&tasks).Error; err != nil {
		return nil, err
	}

	// Ceiling division for total pages.
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	return &ListTasksResult{
		Tasks:      tasks,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetTask fetches one task by id, but ONLY if it belongs to clientID. Scoping by
// client_id is the authorization step: a client can never read another client's
// task. If no row matches (wrong id OR not yours), GORM returns
// gorm.ErrRecordNotFound, which the controller turns into a 404.
func (s *TaskService) GetTask(clientID string, id uint64) (*models.Task, error) {
	var task models.Task
	if err := s.db.Where("id = ? AND client_id = ?", id, clientID).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// CancelTask transitions a task to "cancelled", but only if it's currently
// pending or running (the state-machine rule). Scoped to clientID for ownership.
//
// Note: this reads-then-writes, so there's a small TOCTOU window where a worker
// could pick up the task between our check and save. There are no workers yet
// (Phase 3), and we'll harden this with a row lock / conditional UPDATE then.
func (s *TaskService) CancelTask(clientID string, id uint64) (*models.Task, error) {
	var task models.Task
	if err := s.db.Where("id = ? AND client_id = ?", id, clientID).First(&task).Error; err != nil {
		return nil, err // ErrRecordNotFound -> 404 in the controller
	}

	if task.Status != "pending" && task.Status != "running" {
		return nil, ErrTaskNotCancellable
	}

	now := time.Now()
	task.Status = "cancelled"
	task.CompletedAt = &now // terminal state: record when it ended
	if err := s.db.Save(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}
