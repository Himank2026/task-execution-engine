package watchdog

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Himank2026/task-execution-engine/backend/services"
)

// Watchdog is the LIVENESS guard. On a timer it scans for tasks stuck in "running"
// past a timeout — a worker that hung, or an instance that died without restarting —
// and pushes each back through the failure path so it retries (or dead-letters).
//
// It's the third leg of fault tolerance, covering what the other two miss:
//   - startup recovery (RequeueOrphanedTasks) handles THIS instance's own crash, but
//     only at boot, and only its own tasks;
//   - graceful shutdown drains in-flight work on a clean stop;
//   - the watchdog handles a LIVE instance with a hung task, and (cross-instance) a
//     peer that died and never came back.
type Watchdog struct {
	tasks    *services.TaskService
	interval time.Duration // how often we sweep
	timeout  time.Duration // how long "running" is allowed before we call it stuck

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewWatchdog builds the watchdog. interval and timeout come from config.
func NewWatchdog(tasks *services.TaskService, interval, timeout time.Duration) *Watchdog {
	return &Watchdog{
		tasks:    tasks,
		interval: interval,
		timeout:  timeout,
	}
}

// Start launches the sweep loop and returns immediately.
func (w *Watchdog) Start() {
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.wg.Add(1)
	go w.loop()
	slog.Info("watchdog started", "interval", w.interval.String(), "timeout", w.timeout.String())
}

// Stop ends the loop and waits for the current sweep to finish.
func (w *Watchdog) Stop() {
	slog.Info("watchdog stopping")
	w.cancel()
	w.wg.Wait()
	slog.Info("watchdog stopped")
}

// loop runs a sweep on every tick until shutdown. We run the sweep inline (not in a
// goroutine per tick): a sweep is quick, and if one ever runs long the ticker just
// drops the ticks it missed, so sweeps never overlap.
func (w *Watchdog) loop() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.sweep()
		}
	}
}

// sweep finds every stuck task and routes it through FailTask, which retries it (or
// dead-letters it once retries are exhausted). Treating a timeout as a failure — and
// letting FailTask bump retry_count — is deliberate: a task that hangs EVERY time
// can't be requeued forever, it eventually dead-letters instead of livelocking.
func (w *Watchdog) sweep() {
	stale, err := w.tasks.FindStaleRunningTasks(w.timeout)
	if err != nil {
		slog.Error("watchdog: find stale tasks", "err", err)
		return
	}
	if len(stale) == 0 {
		return
	}

	for i := range stale {
		// Bail promptly if we're shutting down mid-sweep.
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		t := &stale[i]
		reason := fmt.Sprintf("task timed out: stuck in 'running' beyond %s (watchdog)", w.timeout)
		if err := w.tasks.FailTask(t, reason); err != nil {
			slog.Error("watchdog: recover stuck task", "task_id", t.ID, "err", err)
			continue
		}
		slog.Warn("watchdog: recovered stuck task",
			"task_id", t.ID, "retry_count", t.RetryCount, "max_retries", t.MaxRetries,
			"processed_by", t.ProcessedBy)
	}
}
