package worker

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/Himank2026/task-execution-engine/backend/models"
	"github.com/Himank2026/task-execution-engine/backend/services"
)

// Pool is the EXECUTION half of the system: N worker goroutines that run whatever
// tasks they're handed over a channel. It deliberately knows nothing about WHICH
// task to run next or fairness — that's the scheduler's job. The scheduler calls
// Submit() to feed tasks in; the workers just run them.
//
//	scheduler ──Submit()──▶ queue chan ──▶ worker 1
//	                                    └─▶ worker 2
//	                                    └─▶ worker N
//
// (In Phase 3 the pool had its own dispatcher that claimed tasks directly. Phase 4
// moved that decision out to the scheduler so we can schedule fairly; the pool is
// now purely "run what I'm given".)
type Pool struct {
	tasks      *services.TaskService // used to record results (complete/fail)
	instanceID string                // for logging which instance ran it
	numWorkers int
	pub        Publisher // notified on each task state change (may be nil)

	queue  chan *models.Task  // scheduler → workers
	wg     sync.WaitGroup     // tracks workers so Stop() can wait for them
	ctx    context.Context    // cancelled by cancel(); handlers + Submit watch it
	cancel context.CancelFunc // flips ctx to Done() on shutdown

	// states is the live status of each worker (index = workerID-1), so the dashboard
	// can show what each worker is running right now. Guarded by statesMu.
	statesMu sync.Mutex
	states   []workerState
}

// workerState is one worker's in-memory live status.
type workerState struct {
	busy     bool
	taskID   uint64
	taskType string
	clientID string
	since    time.Time
}

// WorkerStatus is the public snapshot of a worker for the /api/workers endpoint.
type WorkerStatus struct {
	ID       int    `json:"id"`
	Busy     bool   `json:"busy"`
	TaskID   uint64 `json:"task_id,omitempty"`
	TaskType string `json:"task_type,omitempty"`
	ClientID string `json:"client_id,omitempty"`
	BusyMs   int64  `json:"busy_ms,omitempty"` // how long it's been on this task
}

// Publisher is the slice of the SSE hub the pool needs: announce that a task changed
// state. The pool depends on this tiny interface (not the concrete hub) so it stays
// decoupled from the sse package — same pattern as the scheduler's Dispatcher.
type Publisher interface {
	PublishTaskEvent(eventType string, taskID uint64, clientID, status string)
}

// NewPool builds the pool but does NOT start it — call Start(). pub may be nil (then
// no events are published).
func NewPool(tasks *services.TaskService, instanceID string, numWorkers int, pub Publisher) *Pool {
	if numWorkers < 1 {
		numWorkers = 1
	}
	return &Pool{
		tasks:      tasks,
		instanceID: instanceID,
		numWorkers: numWorkers,
		pub:        pub,
		states:     make([]workerState, numWorkers),
		// Buffered to numWorkers so the scheduler can stage a little work ahead
		// without blocking on every single Submit.
		queue: make(chan *models.Task, numWorkers),
	}
}

// InstanceID reports which backend instance this pool belongs to (for the dashboard).
func (p *Pool) InstanceID() string { return p.instanceID }

// WorkerStates returns a snapshot of what every worker is doing right now.
func (p *Pool) WorkerStates() []WorkerStatus {
	p.statesMu.Lock()
	defer p.statesMu.Unlock()

	out := make([]WorkerStatus, len(p.states))
	for i, st := range p.states {
		ws := WorkerStatus{ID: i + 1, Busy: st.busy}
		if st.busy {
			ws.TaskID = st.taskID
			ws.TaskType = st.taskType
			ws.ClientID = st.clientID
			ws.BusyMs = time.Since(st.since).Milliseconds()
		}
		out[i] = ws
	}
	return out
}

// setBusy / setIdle record a worker's current state (called as it picks up / finishes
// a task) so WorkerStates can report it live.
func (p *Pool) setBusy(workerID int, task *models.Task) {
	p.statesMu.Lock()
	p.states[workerID-1] = workerState{
		busy: true, taskID: task.ID, taskType: task.Type, clientID: task.ClientID, since: time.Now(),
	}
	p.statesMu.Unlock()
}

func (p *Pool) setIdle(workerID int) {
	p.statesMu.Lock()
	p.states[workerID-1] = workerState{busy: false}
	p.statesMu.Unlock()
}

// publish sends a task event if a publisher is wired (nil-safe).
func (p *Pool) publish(eventType string, task *models.Task, status string) {
	if p.pub != nil {
		p.pub.PublishTaskEvent(eventType, task.ID, task.ClientID, status)
	}
}

// Start spins up the N workers and returns immediately. They sit waiting for tasks
// the scheduler will Submit.
func (p *Pool) Start() {
	p.ctx, p.cancel = context.WithCancel(context.Background())

	for i := 0; i < p.numWorkers; i++ {
		p.wg.Add(1)
		go p.worker(i + 1)
	}

	slog.Info("worker pool started", "workers", p.numWorkers, "instance", p.instanceID)
}

// Submit hands a task to the workers. It blocks if all workers are busy and the
// buffer is full — that natural backpressure is what paces the scheduler. Returns
// false if the pool is shutting down (so the caller knows to stop submitting).
func (p *Pool) Submit(task *models.Task) bool {
	select {
	case p.queue <- task:
		return true
	case <-p.ctx.Done():
		return false
	}
}

// Stop performs a GRACEFUL drain: it stops accepting new work and lets the workers
// finish everything already buffered or in-flight, then exits. We close the queue
// (rather than cancelling the context) so workers drain the buffer and finish their
// current task naturally — nothing in-flight is cut off, so no task is lost or
// wrongly counted as a retry.
//
// Closing the queue is safe ONLY because the scheduler is stopped before us (see the
// defer order in main.go), so nothing can Submit after the close and panic.
func (p *Pool) Stop() {
	slog.Info("worker pool stopping, draining in-flight tasks")
	close(p.queue) // no more new work; workers will drain what's left
	p.wg.Wait()    // block until every worker has finished and returned
	p.cancel()     // all workers done — release the context
	slog.Info("worker pool stopped")
}

// worker is one consumer. It ranges over the queue: it runs tasks as they arrive and
// exits automatically once the queue is closed AND fully drained — that's what makes
// Stop's clean drain work. We deliberately don't watch ctx here, so a task that's
// already running is allowed to finish on shutdown instead of being cut off.
func (p *Pool) worker(id int) {
	defer p.wg.Done()
	for task := range p.queue {
		p.process(id, task)
	}
}

// process runs a single task end-to-end: pick its handler, run it, then record the
// result (complete on success, fail/retry/dead-letter on error).
func (p *Pool) process(workerID int, task *models.Task) {
	p.setBusy(workerID, task)
	defer p.setIdle(workerID) // mark idle again no matter how this returns

	// Stamp started_at now — when the handler actually begins — so execution time
	// reflects pure handler runtime, not the time spent waiting for a free worker.
	if err := p.tasks.MarkStarted(task.ID); err != nil {
		slog.Error("mark started failed", "task_id", task.ID, "err", err)
	}

	slog.Info("task started",
		"worker", workerID, "task_id", task.ID, "client", task.ClientID,
		"type", task.Type, "priority", task.Priority)
	p.publish("task.started", task, "running")

	handler := handlerFor(task.Type)
	err := runHandler(p.ctx, handler, task)

	if err != nil {
		if ferr := p.tasks.FailTask(task, err.Error()); ferr != nil {
			slog.Error("recording failure failed", "task_id", task.ID, "err", ferr)
			return
		}
		slog.Warn("task failed",
			"worker", workerID, "task_id", task.ID,
			"retry_count", task.RetryCount, "max_retries", task.MaxRetries, "err", err)
		p.publish("task.failed", task, "failed")
		return
	}

	if cerr := p.tasks.CompleteTask(task.ID); cerr != nil {
		slog.Error("recording completion failed", "task_id", task.ID, "err", cerr)
		return
	}
	slog.Info("task completed", "worker", workerID, "task_id", task.ID, "client", task.ClientID)
	p.publish("task.completed", task, "completed")
}

// runHandler runs a task's handler behind a panic safety net. Without this, a panic
// in handler code would crash the ENTIRE process — an unrecovered panic in any
// goroutine takes the whole program down, not just that one worker. We recover it,
// log it (with a stack trace for debugging), and turn it into a normal error so the
// task follows the usual fail → retry → dead-letter path, and the worker lives on to
// run the next task.
//
// The named return value (err) is what lets the deferred recover hand an error back
// to the caller in place of the panic.
func runHandler(ctx context.Context, handler Handler, task *models.Task) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("recovered from handler panic",
				"task_id", task.ID, "panic", r, "stack", string(debug.Stack()))
			err = fmt.Errorf("handler panicked: %v", r)
		}
	}()
	return handler(ctx, task)
}
