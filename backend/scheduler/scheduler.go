package scheduler

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Himank2026/task-execution-engine/backend/models"
	"github.com/Himank2026/task-execution-engine/backend/services"
)

// tickInterval is how often the scheduler wakes to do a fair dispatch pass.
const tickInterval = 250 * time.Millisecond

// Dispatcher is the bit of the worker pool the scheduler depends on: hand a task
// to a worker. We depend on this small interface (not the concrete *worker.Pool)
// so the scheduler stays decoupled from the worker package.
type Dispatcher interface {
	Submit(task *models.Task) bool // returns false if the pool is shutting down
}

// Scheduler is the FAIRNESS half of the system. On a timer it runs a Deficit Round
// Robin pass: it cycles through the clients that have pending work and lets each
// dispatch up to a quantum of tasks per round, so one busy client can't starve the
// others. Priority is still honored WITHIN a client's turn (the per-client dequeue
// is ORDER BY priority).
//
// It replaces Phase 3's "claim the globally-next task" dispatcher: fairness is
// decided here in Go (which client), priority is decided in SQL (which task).
type Scheduler struct {
	tasks      *services.TaskService
	pool       Dispatcher
	instanceID string
	quantum    int // tasks each client may dispatch per round

	// deficits carries each client's leftover quota across rounds (the "deficit"
	// in Deficit Round Robin). Only ever touched by the one pass that holds the
	// isScheduling guard, so it needs no separate lock.
	deficits map[string]int

	// isScheduling guards against overlapping passes: each tick fires a pass in its
	// own goroutine, so if a pass is still running (e.g. it blocked on a full worker
	// queue), the next tick's pass sees this flag and bows out instead of stacking
	// up. Exactly one pass runs at a time.
	mu           sync.Mutex
	isScheduling bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewScheduler builds the scheduler. quantum < 1 is clamped to 1.
func NewScheduler(tasks *services.TaskService, pool Dispatcher, instanceID string, quantum int) *Scheduler {
	if quantum < 1 {
		quantum = 1
	}
	return &Scheduler{
		tasks:      tasks,
		pool:       pool,
		instanceID: instanceID,
		quantum:    quantum,
		deficits:   make(map[string]int),
	}
}

// Start launches the ticking loop and returns immediately.
func (s *Scheduler) Start() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.wg.Add(1)
	go s.loop()
	slog.Info("scheduler started", "quantum", s.quantum, "interval", tickInterval.String())
}

// Stop cancels the loop and waits for any in-flight pass to finish.
func (s *Scheduler) Stop() {
	slog.Info("scheduler stopping")
	s.cancel()
	s.wg.Wait()
	slog.Info("scheduler stopped")
}

// loop wakes on each tick and fires a scheduling pass in its own goroutine. Running
// the pass in a goroutine (rather than inline) is what makes the isScheduling guard
// meaningful: a slow pass can't delay the next tick, and overlapping passes are
// skipped by the guard.
func (s *Scheduler) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.scheduleOnce()
			}()
		}
	}
}

// scheduleOnce is one Deficit Round Robin pass: each active client gets a quantum
// added to its deficit, then dispatches tasks (highest priority first) until its
// deficit runs out or it has no pending tasks left.
func (s *Scheduler) scheduleOnce() {
	// --- the isScheduling guard: only one pass at a time ---
	s.mu.Lock()
	if s.isScheduling {
		s.mu.Unlock()
		return // a previous pass is still running; skip this tick
	}
	s.isScheduling = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.isScheduling = false
		s.mu.Unlock()
	}()

	// Which clients have pending work right now?
	clients, err := s.tasks.ClientsWithPendingTasks()
	if err != nil {
		slog.Error("scheduler: list clients", "err", err)
		return
	}
	if len(clients) == 0 {
		// Idle: drop all deficits so a client that goes quiet can't hoard credit
		// and jump the queue when it comes back.
		s.deficits = make(map[string]int)
		return
	}

	// Deterministic round order so fairness is stable and testable.
	sort.Strings(clients)

	// Forget deficits for clients that no longer have work (classic DRR: a flow that
	// empties its queue resets to 0).
	active := make(map[string]bool, len(clients))
	for _, c := range clients {
		active[c] = true
	}
	for c := range s.deficits {
		if !active[c] {
			delete(s.deficits, c)
		}
	}

	// One turn per client this round.
	for _, c := range clients {
		s.deficits[c] += s.quantum

		for s.deficits[c] > 0 {
			// Abort promptly on shutdown.
			select {
			case <-s.ctx.Done():
				return
			default:
			}

			task, err := s.tasks.DequeueTaskForClient(c, s.instanceID)
			if err != nil {
				if err == services.ErrNoTask {
					// This client's queue is empty — reset its deficit (no hoarding)
					// and move to the next client.
					s.deficits[c] = 0
					break
				}
				slog.Error("scheduler: dequeue", "client", c, "err", err)
				break
			}

			// Hand it to a worker. Submit blocks if all workers are busy (that's the
			// backpressure that paces us); returns false only on shutdown.
			if !s.pool.Submit(task) {
				return
			}
			s.deficits[c]--
		}
	}
}
