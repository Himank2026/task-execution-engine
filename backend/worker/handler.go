package worker

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/Himank2026/task-execution-engine/backend/models"
)

// failureRate is the chance a simulated task "fails", so we exercise the retry +
// dead-letter paths. Tunable: raise it to see more retries/dead-letters.
const failureRate = 0.2

// Handler runs the actual work for a task. A non-nil return means the task failed
// (and may be retried). The context lets us abort work on graceful shutdown.
type Handler func(ctx context.Context, task *models.Task) error

// simulatedWork fakes real work: sleep a random time, then randomly succeed or
// fail. The select on ctx.Done() means an in-flight task stops promptly if the
// app is shutting down instead of blocking the whole sleep.
func simulatedWork(ctx context.Context, task *models.Task) error {
	d := time.Duration(500+rand.Intn(2500)) * time.Millisecond // 0.5s–3s
	select {
	case <-time.After(d):
		// finished "working"
	case <-ctx.Done():
		return ctx.Err()
	}

	if rand.Float64() < failureRate {
		return errors.New("simulated task failure")
	}
	return nil
}

// handlerFor returns the handler for a task type. Today every type uses the same
// simulated handler; this is the seam where real per-type handlers (send_email,
// resize_image, ...) would be registered.
func handlerFor(taskType string) Handler {
	return simulatedWork
}
