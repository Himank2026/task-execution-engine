package services

import (
	"encoding/json"
	"errors"
	"math/rand"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Himank2026/task-execution-engine/backend/models"
)

// ErrTaskNotCancellable is returned when a cancel is attempted on a task that has
// already reached a terminal state (completed/failed/cancelled). The controller
// maps this to HTTP 409 Conflict.
var ErrTaskNotCancellable = errors.New("task is not in a cancellable state")

// ErrNoTask signals that the queue currently has no pending task. It's a normal
// condition, not a failure — a worker that gets this just waits and tries again.
var ErrNoTask = errors.New("no pending task available")

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

// BatchCreateTasks inserts many tasks for a client in ONE bulk insert — far cheaper
// than N separate round-trips when submitting a burst.
func (s *TaskService) BatchCreateTasks(clientID string, inputs []CreateTaskInput) (int, error) {
	tasks := make([]models.Task, 0, len(inputs))
	for _, in := range inputs {
		t := models.Task{
			ClientID:   clientID,
			Type:       in.Type,
			Priority:   in.Priority,
			Payload:    in.Payload,
			Status:     "pending",
			MaxRetries: 3,
		}
		if in.MaxRetries != nil {
			t.MaxRetries = *in.MaxRetries
		}
		tasks = append(tasks, t)
	}
	if err := s.db.Create(&tasks).Error; err != nil {
		return 0, err
	}
	return len(tasks), nil
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
	// AllClients, when true, skips the per-client scoping (ops/dashboard view).
	AllClients bool
	// FilterClient, when set, narrows the list to one specific client (used by the
	// dashboard's client filter on top of the all-clients view).
	FilterClient string
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
	// Base query: scoped to this client, UNLESS the caller asked for all clients
	// (the dashboard's ops view).
	q := s.db.Model(&models.Task{})
	if !f.AllClients {
		q = q.Where("client_id = ?", clientID)
	}

	// Add a WHERE only for filters that were actually provided.
	if f.FilterClient != "" {
		q = q.Where("client_id = ?", f.FilterClient)
	}
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

// DequeueTaskForClient atomically claims the next pending task FOR ONE CLIENT and
// marks it running. It is the concurrency-safe heart of the system:
//
//	BEGIN
//	  SELECT ... FOR UPDATE SKIP LOCKED   -- claim one row, skip rows others hold
//	  UPDATE ... SET status='running'     -- flip it, stamp who's running it
//	COMMIT
//
// Because the SELECT locks the row and SKIP LOCKED steps over already-claimed rows,
// no two workers (or backend instances) ever grab the same task. The lock only
// exists inside the transaction, which is why both statements run in one.
//
// It's scoped to a single client because the DRR scheduler decides FAIRNESS by
// choosing which client to call this for; priority (ORDER BY) still rules WITHIN
// that client's turn. Returns ErrNoTask when this client has no pending task.
func (s *TaskService) DequeueTaskForClient(clientID, instanceID string) (*models.Task, error) {
	var task models.Task

	err := s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Raw(`
			SELECT id, client_id, type, priority, payload, status, retry_count, max_retries, created_at
			FROM tasks
			WHERE status = 'pending' AND client_id = ?
			ORDER BY priority DESC, created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		`, clientID).Scan(&task)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNoTask // this client has no pending task right now
		}

		// Mark it running + stamp the owner, but DON'T set started_at here. started_at is
		// set when a WORKER actually begins the handler (MarkStarted), so "execution
		// time" is the pure handler runtime and "queue wait" is the full time until work
		// began (including any brief in-buffer wait under load).
		if err := tx.Model(&models.Task{}).
			Where("id = ?", task.ID).
			Updates(map[string]any{
				"status":       "running",
				"processed_by": instanceID,
			}).Error; err != nil {
			return err
		}

		task.Status = "running"
		task.ProcessedBy = &instanceID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// ClientsWithPendingTasks returns the distinct client_ids that currently have at
// least one pending task. The scheduler uses this each round to know which clients
// are "active" (have work) and therefore deserve a turn.
func (s *TaskService) ClientsWithPendingTasks() ([]string, error) {
	var clientIDs []string
	err := s.db.Model(&models.Task{}).
		Where("status = ?", "pending").
		Distinct().
		Pluck("client_id", &clientIDs).Error
	if err != nil {
		return nil, err
	}
	return clientIDs, nil
}

// RequeueOrphanedTasks is startup recovery: if THIS instance crashed last time, it
// may have left tasks stuck in "running" that no worker is actually running anymore.
// We flip them back to "pending" and clear the timing/owner fields so they get
// cleanly re-claimed. Crucially we do NOT bump retry_count — the task didn't fail,
// the instance died, so it shouldn't be penalized.
//
// Scoped to processed_by = instanceID so we only reclaim OUR own orphans — in a
// multi-instance deployment we must not steal tasks another live instance is
// legitimately running. (Cross-instance hang detection is the watchdog's job, Phase 5.)
func (s *TaskService) RequeueOrphanedTasks(instanceID string) (int64, error) {
	res := s.db.Model(&models.Task{}).
		Where("status = ? AND processed_by = ?", "running", instanceID).
		Updates(map[string]any{
			"status":       "pending",
			"started_at":   nil,
			"processed_by": nil,
		})
	return res.RowsAffected, res.Error
}

// FindStaleRunningTasks returns tasks stuck in "running" longer than timeout. A
// healthy task finishes well within the timeout, so anything older than (now -
// timeout) signals its worker hung or its instance died. The watchdog calls this on a
// timer and then routes each one back through the failure path.
//
// Unlike RequeueOrphanedTasks (scoped to one instance, runs once at boot), this is
// NOT scoped to an instance: a peer instance that died and never restarted leaves
// stuck tasks only a *different* instance's watchdog can rescue. The timeout — set
// comfortably larger than the longest real task — is what keeps us from reclaiming a
// task that's merely slow but still alive.
func (s *TaskService) FindStaleRunningTasks(timeout time.Duration) ([]models.Task, error) {
	cutoff := time.Now().Add(-timeout)
	var tasks []models.Task
	err := s.db.
		Where("status = ? AND started_at < ?", "running", cutoff).
		Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// AddDemoTasks is a DEV/DEMO helper: it APPENDS n fresh random PENDING tasks spread
// across all active clients. It does NOT wipe — clicking "Demo data" again adds another
// batch, so the totals grow (50 → 100 → 150). Use ClearAllData to reset to empty.
// Spreading across clients makes the multi-tenant behaviour visible (mix of
// alpha/beta/gamma in the worker panel and task list).
func (s *TaskService) AddDemoTasks(n int) error {
	demoTypes := []string{"send_email", "send_sms", "generate_report", "resize_image"}

	// Which clients exist? Spread the demo tasks across them.
	var clientIDs []string
	if err := s.db.Model(&models.APIKey{}).Where("active = ?", true).Pluck("client_id", &clientIDs).Error; err != nil {
		return err
	}
	if len(clientIDs) == 0 {
		clientIDs = []string{"alpha"} // fallback so the demo still works
	}

	tasks := make([]models.Task, 0, n)
	for i := 0; i < n; i++ {
		tasks = append(tasks, models.Task{
			ClientID:   clientIDs[rand.Intn(len(clientIDs))],
			Type:       demoTypes[rand.Intn(len(demoTypes))],
			Priority:   uint8(rand.Intn(5) + 1),
			Status:     "pending",
			MaxRetries: 3,
		})
	}
	return s.db.Create(&tasks).Error
}

// MarkStarted stamps started_at at the moment a worker actually begins running the
// task (vs when the scheduler claimed it). This makes execution time = pure handler
// runtime, and queue wait = the full time from submission until work began.
func (s *TaskService) MarkStarted(id uint64) error {
	return s.db.Model(&models.Task{}).Where("id = ?", id).Update("started_at", time.Now()).Error
}

// ClearAllData wipes every task and dead-letter row — the dashboard's "Clear" button
// uses it to reset to an empty slate. DEV/DEMO utility.
func (s *TaskService) ClearAllData() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&models.Task{}).Error; err != nil {
			return err
		}
		return tx.Where("1 = 1").Delete(&models.DeadLetterTask{}).Error
	})
}

// CompleteTask marks a successfully-run task as completed. completed_at is set so
// throughput analytics (completions per time window) have a timestamp to count.
func (s *TaskService) CompleteTask(id uint64) error {
	now := time.Now()
	return s.db.Model(&models.Task{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       "completed",
			"completed_at": now,
		}).Error
}

// FailTask handles a task whose handler returned an error. It forks:
//   - retries left  -> bump retry_count, put it back to "pending", CLEAR the
//     timing fields so the next attempt times cleanly (the retry rule).
//   - retries exhausted -> dead-letter it (see deadLetter).
//
// The fork uses the task's current retry_count, so pass the struct you got from
// DequeueTask (it carries the up-to-date counts).
func (s *TaskService) FailTask(task *models.Task, taskErr string) error {
	if task.RetryCount < task.MaxRetries {
		// Retry: back into the queue for another attempt. map-based Updates lets us
		// write NULL into started_at/completed_at (struct Updates would skip them).
		return s.db.Model(&models.Task{}).
			Where("id = ?", task.ID).
			Updates(map[string]any{
				"status":        "pending",
				"retry_count":   task.RetryCount + 1,
				"error_message": taskErr,
				"started_at":    nil,
				"completed_at":  nil,
			}).Error
	}
	return s.deadLetter(task, taskErr)
}

// deadLetter retires a task that has failed too many times. In one transaction it
// (1) marks the original row "failed" with completed_at (keeps the tasks table as
// the analytics source of truth) and (2) copies it into dead_letter_queue for
// later inspection / manual retry. Doing both in a transaction keeps them
// consistent — we never end up with one without the other.
func (s *TaskService) deadLetter(task *models.Task, taskErr string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		if err := tx.Model(&models.Task{}).
			Where("id = ?", task.ID).
			Updates(map[string]any{
				"status":        "failed",
				"error_message": taskErr,
				"completed_at":  now,
			}).Error; err != nil {
			return err
		}

		dlq := models.DeadLetterTask{
			OriginalTaskID: task.ID,
			ClientID:       task.ClientID,
			Type:           task.Type,
			Priority:       task.Priority,
			Payload:        task.Payload,
			RetryCount:     task.RetryCount,
			MaxRetries:     task.MaxRetries,
			FinalError:     &taskErr,
			ProcessedBy:    task.ProcessedBy,
			CreatedAt:      task.CreatedAt, // preserve original submission time
			FailedAt:       now,
		}
		return tx.Create(&dlq).Error
	})
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
// This is a single atomic check-and-set: the WHERE clause GUARDS the transition
// (status IN ('pending','running')) and the UPDATE applies it, all in one locked
// operation. That closes the TOCTOU race the old read-then-save version had — now
// that workers exist, a worker can no longer slip in and claim the row between our
// check and our write, because there's no gap: the DB checks and writes together.
func (s *TaskService) CancelTask(clientID string, id uint64) (*models.Task, error) {
	now := time.Now()

	// One guarded UPDATE. RowsAffected tells us whether the guard let it through.
	res := s.db.Model(&models.Task{}).
		Where("id = ? AND client_id = ? AND status IN ?", id, clientID, []string{"pending", "running"}).
		Updates(map[string]any{
			"status":       "cancelled",
			"completed_at": now, // terminal state: record when it ended
		})
	if res.Error != nil {
		return nil, res.Error
	}

	if res.RowsAffected == 1 {
		// Cancelled. Read the fresh row back so the caller gets the updated task.
		var task models.Task
		if err := s.db.Where("id = ? AND client_id = ?", id, clientID).First(&task).Error; err != nil {
			return nil, err
		}
		return &task, nil
	}

	// RowsAffected == 0: the guard rejected the update. Figure out WHY so the
	// controller can return the right status code.
	var task models.Task
	if err := s.db.Where("id = ? AND client_id = ?", id, clientID).First(&task).Error; err != nil {
		return nil, err // ErrRecordNotFound -> 404 (wrong id, or not this client's)
	}
	// It exists and is ours, so the only reason the guard failed is that it was
	// already in a terminal state (completed/failed/cancelled).
	return nil, ErrTaskNotCancellable
}
