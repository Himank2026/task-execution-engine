package worker

import (
	"context"
	"log/slog"
	"sync"

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

	queue  chan *models.Task  // scheduler → workers
	wg     sync.WaitGroup     // tracks workers so Stop() can wait for them
	ctx    context.Context    // cancelled by cancel(); handlers + Submit watch it
	cancel context.CancelFunc // flips ctx to Done() on shutdown
}

// NewPool builds the pool but does NOT start it — call Start().
func NewPool(tasks *services.TaskService, instanceID string, numWorkers int) *Pool {
	if numWorkers < 1 {
		numWorkers = 1
	}
	return &Pool{
		tasks:      tasks,
		instanceID: instanceID,
		numWorkers: numWorkers,
		// Buffered to numWorkers so the scheduler can stage a little work ahead
		// without blocking on every single Submit.
		queue: make(chan *models.Task, numWorkers),
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

// Stop signals shutdown and waits for all workers to finish. We cancel the context
// (workers stop pulling, in-flight handlers abort) rather than closing the channel,
// so a late Submit can never panic on a closed channel.
//
// Contract: stop the SCHEDULER before calling this, so nothing is still Submitting.
func (p *Pool) Stop() {
	slog.Info("worker pool stopping")
	p.cancel()
	p.wg.Wait()
	slog.Info("worker pool stopped")
}

// worker is one consumer. It pulls tasks until the context is cancelled. We select
// on ctx.Done() too (not just range) so a worker can exit promptly on shutdown even
// if no task ever arrives.
func (p *Pool) worker(id int) {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case task := <-p.queue:
			p.process(id, task)
		}
	}
}

// process runs a single task end-to-end: pick its handler, run it, then record the
// result (complete on success, fail/retry/dead-letter on error).
func (p *Pool) process(workerID int, task *models.Task) {
	slog.Info("task started",
		"worker", workerID, "task_id", task.ID, "client", task.ClientID,
		"type", task.Type, "priority", task.Priority)

	handler := handlerFor(task.Type)
	err := handler(p.ctx, task)

	if err != nil {
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
	slog.Info("task completed", "worker", workerID, "task_id", task.ID, "client", task.ClientID)
}
