# 03 — Backend (Go)

This doc describes the Go backend: how it's organized, how the worker pool and scheduler work, and how concurrency is handled. This is the **star of the project** — the part interviewers grill.

---

## Project structure

We use a **flat, layer-per-folder** layout: `main.go` sits at the backend root, and each layer/concern is its own folder (= one Go package). Within each folder, one file per feature (e.g. `controllers/task.go`, `services/task.go`). The request flow is **main → routes → controllers → services → models**.

```
backend/
  main.go                  ← entry point: load config, connect DB/Redis, start API + workers
  config/                  ← env var config with defaults
  database/
    mysql.go               ← MySQL connection pool (database/sql + driver)
    redis.go               ← Redis client (go-redis) + error handling
    migrations/            ← *.sql schema files
    seed.go                ← generates ~60 test tasks across clients
  models/                  ← shared structs: Task, SSEEvent, WorkerMessage, etc.
  routes/
    routes.go              ← Gin engine setup + route registration
    task.go                ← task route group        (Phase 2)
  controllers/
    health.go              ← health check
    task.go                ← task CRUD + cancel + retry + DLQ endpoints
    analytics.go           ← analytics endpoints (4 chart queries)
    sse.go                 ← SSE stream endpoint
  services/
    task.go                ← create / dequeue / complete / fail / retry / requeue / cancel
    analytics.go           ← aggregate queries for charts
  middleware/
    apikey.go              ← API key auth (Redis-cached ~5min)
    ratelimit.go           ← sliding-window rate limit (Redis + in-memory fallback)
    errors.go              ← centralized error handling / JSON error responses
  scheduler/
    scheduler.go           ← Deficit Round Robin loop; isScheduling guard
  worker/
    pool.go                ← worker pool: spawn, crash recovery, hung-worker watchdog
    worker.go              ← a single worker: pull task, run handler, report result
    handlers/              ← task handlers (simulated work) keyed by task type
  sse/
    hub.go                 ← connection registry (map id→client), broadcast, heartbeat
  Dockerfile
  go.mod / go.sum
```

> **Layout notes:** Each folder is one Go package; cross-package calls use exported (Capitalized) names. `main.go` stays thin — it only wires dependencies and starts the server. SQL currently lives in `services/`; a dedicated `repository/` layer (isolating SQL) is an optional refinement we may add in Phase 2 if services grow. Most folders are created as we reach the phase that needs them, not up front.

## The concurrency model (Go's superpower here)

The Node version used `worker_threads`. In Go it's cleaner: **goroutines + channels.**

- **Goroutine** = a super-lightweight thread managed by Go (you can run thousands; each costs ~KBs). Started with `go someFunction()`.
- **Channel** = a typed pipe goroutines use to safely pass data/signals to each other. The Go mantra: *"don't communicate by sharing memory; share memory by communicating."*

### Worker pool design

```
                 ┌─────────────────────────────────────────┐
   Scheduler ───▶│ tasks channel  (chan Task, buffered)     │
                 └───────────┬─────────────────────────────-┘
                  ┌──────────┼──────────┬──────────┐
              ┌───▼──┐   ┌───▼──┐   ┌───▼──┐   ┌───▼──┐
              │ w1   │   │ w2   │   │ w3   │   │ wN   │   ← N worker goroutines
              └───┬──┘   └───┬──┘   └───┬──┘   └───┬──┘
                  └──────────┴──────────┴──────────┘
                              │ each: run handler, report result
                              ▼
                 ┌─────────────────────────────┐
                 │ results channel ─► DB update │ ─► SSE push
                 └─────────────────────────────┘
```

- The pool spawns **N worker goroutines** (configurable, e.g. 8).
- Each worker loops: receive a task from the channel → run its handler → write the result.
- The number of workers = your concurrency level (a resume metric).

### Crash recovery

Each worker is supervised. If a worker goroutine panics/dies:
1. A `recover()` (Go's panic-catcher) or the supervising goroutine detects the exit.
2. The in-flight task is **requeued** (back to `pending`) so it isn't lost.
3. A **replacement worker** is spawned to keep the pool at full strength.

> Design rule: cleanup/requeue logic lives in **one place** (the supervisor), never duplicated — like the Node note "terminate triggers exit; don't duplicate cleanup."

### Hung-worker watchdog

A watchdog goroutine wakes every ~15s and checks for tasks stuck in `running` longer than a timeout (~60s). Those are assumed hung; the task is requeued and the worker recycled. *Metric: "detected & recovered stuck workers within Xs."*

## The scheduler — Deficit Round Robin (DRR)

**Problem it solves:** pure priority/FIFO lets one client hog all workers (starvation).

**How DRR works (simplified):**
- Each client has a "deficit counter" (a quota of how much work it can dispatch this round).
- The scheduler cycles through clients; each round, a client may dispatch tasks up to its quota, then it's the next client's turn.
- A client with nothing to do passes its turn; busy clients still take turns rather than monopolize.

**Result:** fair progress for everyone, while still honoring priority *within* each client's share.

**Guard:** an `isScheduling` flag prevents two scheduler ticks from overlapping (async ticks must not stack up). One tick finishes before the next starts.

## Task handlers (the simulated work)

Each task `type` maps to a handler function. For the demo, handlers:
- Sleep a random duration (simulating real work like encoding/emailing).
- Randomly succeed or throw an error (so we can demonstrate retries, DLQ, failure-rate charts).

```go
// pseudo-shape
func EmailHandler(ctx context.Context, t Task) error {
    time.Sleep(randomDuration())
    if rand.Float64() < failureRate {
        return fmt.Errorf("simulated failure")
    }
    return nil
}
```

> **The reusability point (say this in interviews):** "To make it real, I'd replace the handler body with actual work — sending an email, encoding a video — and the entire engine around it stays identical."

## API endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/tasks` | Submit a task `{type, priority, payload}` |
| GET | `/api/tasks` | Query (status, priority, type, client_id, date range, pagination, sorting) |
| GET | `/api/tasks/:id` | Single task |
| GET | `/api/tasks/dlq` | Dead letter queue |
| POST | `/api/tasks/:id/cancel` | Cancel pending/running |
| POST | `/api/tasks/:id/retry` | Retry a dead-lettered task |
| GET | `/api/sse/events` | SSE live stream |
| GET | `/api/analytics/execution-time` | Avg execution time |
| GET | `/api/analytics/throughput` | Tasks completed over time |
| GET | `/api/analytics/failure-rate` | Failure % over time |
| GET | `/api/analytics/queue-wait` | Avg queue wait time |
| GET | `/api/health` | Health + worker status |
| POST | `/api/seed` | Seed test tasks |

## Graceful shutdown + startup recovery

- **Startup:** requeue any orphaned `running` tasks (left over from a previous crash) using `requeueTask()`.
- **Shutdown:** on SIGTERM/SIGINT, stop accepting new tasks, let in-flight workers finish (with a timeout), close DB/Redis cleanly. (Go: `context.Context` cancellation + `http.Server.Shutdown`.)

## Suggested Go libraries

| Need | Library |
|------|---------|
| HTTP router | `github.com/gin-gonic/gin` |
| MySQL driver | `github.com/go-sql-driver/mysql` (with stdlib `database/sql`) |
| Redis | `github.com/redis/go-redis/v9` |
| Config from env | stdlib `os` + `github.com/joho/godotenv` (dev only) |
| Logging | stdlib `log/slog` (structured JSON) |
| UUIDs (instance id) | `github.com/google/uuid` |
| Testing | stdlib `testing` + `github.com/stretchr/testify` |

→ Next: [04 — Frontend](04-FRONTEND.md)
