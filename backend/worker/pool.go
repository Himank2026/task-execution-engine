package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Himank2026/task-execution-engine/backend/models"
	"github.com/Himank2026/task-execution-engine/backend/services"
)

// pollInterval is how long the dispatcher sleeps when the queue is empty before
// asking the DB again. Short enough to feel responsive, long enough not to hammer
// the database with SELECTs when there's no work.
const pollInterval = 1 * time.Second

// Pool is the worker pool: one dispatcher goroutine that claims tasks from the DB
// and fans them out over a channel to N worker goroutines that run them.
//
//	dispatcher ──(queue chan)──▶ worker 1
//	                          └─▶ worker 2
//	                          └─▶ worker N
//
// The dispatcher is the ONLY thing that calls DequeueTask (the SKIP LOCKED claim),
// so workers never touch the queue directly — they just run whatever lands in the
// channel. This keeps the claim logic in one place.
type Pool struct {
	tasks      *services.TaskService // shared business logic (same instance the API uses)
	instanceID string                // stamped onto processed_by; proves which backend ran it
	numWorkers int

	queue  chan *models.Task  // dispatcher → workers
	wg     sync.WaitGroup     // tracks live goroutines so Stop() can wait for them
	cancel context.CancelFunc // flips the shared context to Done() on shutdown
	ctx    context.Context    // cancelled by cancel(); handlers watch ctx.Done()
}

// NewPool builds a pool but does NOT start it — call Start() for that. The
// taskService is injected (same instance main shares with the HTTP layer) so the
// workers and the API operate on one set of business rules.
func NewPool(tasks *services.TaskService, instanceID string, numWorkers int) *Pool {
	if numWorkers < 1 {
		numWorkers = 1 // a pool with zero workers would never do anything
	}
	return &Pool{
		tasks:      tasks,
		instanceID: instanceID,
		numWorkers: numWorkers,
		// Buffered to numWorkers so the dispatcher can stage a little work ahead
		// without blocking, but never claims far more than the workers can chew.
		queue: make(chan *models.Task, numWorkers),
	}
}

// Start spins up the dispatcher + N workers and returns immediately (they run in
// the background). Call Stop() to shut them down cleanly.
func (p *Pool) Start() {
	p.ctx, p.cancel = context.WithCancel(context.Background())

	// Launch the workers first so they're ready to receive before the dispatcher
	// starts handing out tasks.
	for i := 0; i < p.numWorkers; i++ {
		p.wg.Add(1)
		go p.worker(i + 1)
	}

	// One dispatcher feeds them all.
	p.wg.Add(1)
	go p.dispatch()

	slog.Info("worker pool started", "workers", p.numWorkers, "instance", p.instanceID)
}

// Stop signals shutdown (cancels the context) and blocks until the dispatcher and
// all workers have exited. Safe to call once during graceful shutdown.
func (p *Pool) Stop() {
	slog.Info("worker pool stopping")
	p.cancel()  // tell dispatcher to stop claiming + in-flight handlers to abort
	p.wg.Wait() // wait for everyone to finish
	slog.Info("worker pool stopped")
}

// dispatch is the single producer: loop forever claiming the next pending task and
// pushing it onto the queue channel for a worker to pick up. It is the only caller
// of DequeueTask.
func (p *Pool) dispatch() {
	defer p.wg.Done()
	// When the dispatcher exits, close the channel so the workers' range loops end.
	defer close(p.queue)

	for {
		// Bail out promptly on shutdown.
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		task, err := p.tasks.DequeueTask(p.instanceID)
		if err != nil {
			if err == services.ErrNoTask {
				// Queue empty: wait a beat, but wake early if we're shutting down.
				select {
				case <-time.After(pollInterval):
				case <-p.ctx.Done():
					return
				}
				continue
			}
			// A real DB error: log it and back off briefly rather than spin hot.
			slog.Error("dequeue failed", "err", err)
			select {
			case <-time.After(pollInterval):
			case <-p.ctx.Done():
				return
			}
			continue
		}

		// Hand the claimed task to a worker. If all workers are busy this blocks
		// until one is free (natural backpressure) — but still abort on shutdown.
		select {
		case p.queue <- task:
		case <-p.ctx.Done():
			return
		}
	}
}

// worker is one consumer: range over the channel until it's closed, running each
// task it receives. N of these run concurrently = the pool's concurrency level.
func (p *Pool) worker(id int) {
	defer p.wg.Done()
	for task := range p.queue {
		p.process(id, task)
	}
}

// process runs a single task end-to-end: pick its handler, run it, then record the
// result (complete on success, fail/retry/dead-letter on error).
func (p *Pool) process(workerID int, task *models.Task) {
	slog.Info("task started",
		"worker", workerID, "task_id", task.ID, "type", task.Type, "priority", task.Priority)

	handler := handlerFor(task.Type)
	err := handler(p.ctx, task)

	if err != nil {
		// FailTask decides internally: retry (back to pending) or dead-letter.
		if ferr := p.tasks.FailTask(task, err.Error()); ferr != nil {
			slog.Error("recording failure failed", "task_id", task.ID, "err", ferr)
			return
		}
		slog.Warn("task failed",
			"worker", workerID, "task_id", task.ID,
			"retry_count", task.RetryCount, "max_retries", task.MaxRetries, "err", err)
		return
	}

	if cerr := p.tasks.CompleteTask(task.ID); cerr != nil {
		slog.Error("recording completion failed", "task_id", task.ID, "err", cerr)
		return
	}
	slog.Info("task completed", "worker", workerID, "task_id", task.ID)
}
