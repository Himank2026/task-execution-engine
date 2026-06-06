# 01 — Architecture

This doc explains *how the whole system fits together* and *why* it's built this way. If you understand this doc, you can defend the project in any interview.

---

## The problem we're solving

Some work takes too long to do during a normal web request (imagine encoding a video, generating a 10,000-row report, sending 50,000 emails). You can't make a user's browser wait 5 minutes. The solution: **accept the job, say "got it", and do the work in the background** — then let the user track progress.

That's a **task execution engine** (a.k.a. job queue / background worker system). Real-world examples: Celery (Python), Sidekiq (Ruby), BullMQ (Node). We're building one from scratch in Go.

## The core idea in one paragraph

A **client** submits a **task** via the API. The task is saved to a **MySQL table** with status `pending` (this table *is* the queue). A pool of **worker goroutines** continuously pulls pending tasks, runs them, and updates their status to `running` → `completed` (or `failed` → retried → `dead_letter`). A **scheduler** decides *which* task to hand out next, fairly across clients. Every status change is pushed to the browser live via **SSE**, so the dashboard updates in real time.

## Data flow (follow one task's life)

```
1. Client: POST /api/tasks  {type, priority, payload}  + API key
                │
                ▼
2. API validates key (Redis-cached) + checks rate limit (Redis)
                │
                ▼
3. Task row inserted into MySQL:  status = 'pending'
                │
                ▼
4. Scheduler picks next task (fair, by priority) ──► hands to a free worker
                │
                ▼
5. Worker locks the row (SKIP LOCKED), sets status = 'running'
                │
                ▼
6. Worker runs the task handler (simulated work: sleep + random pass/fail)
                │
        ┌───────┴────────┐
        ▼                ▼
   success: 'completed'  failure: retry_count < max?
                              ├─ yes ─► 'pending' again (retry)
                              └─ no ──► moved to dead_letter_queue
                │
                ▼
7. Every status change ──► SSE push ──► dashboard updates live
```

## Component responsibilities

| Component | Job | Lives in |
|-----------|-----|----------|
| **API (Gin routes + handlers)** | Accept tasks, serve queries, expose SSE stream | `routes/` + `controllers/` |
| **Task service** | Business logic: create/dequeue/complete/fail/retry tasks | `services/` |
| **Scheduler** | Decide which task runs next, fairly (Deficit Round Robin) | `scheduler/` |
| **Worker pool** | Run tasks concurrently; crash recovery; hung detection | `worker/` |
| **Task handlers** | The actual (simulated) work per task type | `worker/` |
| **Rate limiter** | Enforce 10 req/min/client globally | `middleware/` + Redis |
| **SSE hub** | Hold open browser connections, broadcast events | `sse/` |
| **Analytics** | Aggregate queries for the charts | `services/` + `controllers/` |

> **Layout note:** this project uses a **flat, package-per-folder layout** under `backend/` (`main.go` at the root with sibling packages above) — not the Go `cmd/`+`internal/` convention. Models live in `models/`, infra/connection + migration + seed in `database/`. See [03 — Backend](03-BACKEND.md) for the full tree.

## The 5 key design decisions (and the short "why")

1. **Priority: 1–5 where 5 is highest.** SQL orders by `priority DESC, created_at ASC` — high priority first, oldest first within a priority. *Defense: important work shouldn't wait behind trivial work, but within the same priority it's fair (FIFO).*

2. **Fair scheduling via Deficit Round Robin (DRR).** Instead of pure priority (which lets one client starve everyone), the scheduler takes turns across clients. *Defense: prevents starvation — one client dumping 1,000 tasks can't block the others.*

3. **DB-backed queue with `SELECT ... FOR UPDATE SKIP LOCKED`.** The queue is just a MySQL table. The lock ensures two workers never grab the same task. *Defense: simpler than Kafka, and a real, well-known pattern; SKIP LOCKED gives concurrency-safe dequeue.*

4. **Real-time via SSE, not WebSockets.** Data flows one way (server → browser). *Defense: SSE is simpler and sufficient; WebSockets' two-way channel would be unused complexity.*

5. **Redis for rate limiting (with in-memory fallback).** A shared counter across all backend copies; if Redis dies, it degrades to per-instance limiting instead of crashing. *Defense: in-memory counters can't be shared across instances; Redis makes the limit global; the fallback shows graceful degradation.*

## "Why is this distributed?" (the honest version)

**Concurrent** = one program doing many things at once (a single backend with a worker pool of goroutines).
**Distributed** = the work is spread across *multiple separate processes/machines* that coordinate through shared state.

This project is distributed because we run **multiple identical backend copies** behind an nginx load balancer, all coordinating through:
- a **shared MySQL queue** (with `SKIP LOCKED` so no task runs twice), and
- a **shared Redis** (so rate limits are global).

To make it *visible*, each backend copy has an `INSTANCE_ID`, and we record which instance processed each task. The dashboard/logs then show work genuinely spread across copies:

```
Task #41 → backend-2
Task #42 → backend-1
Task #43 → backend-3
```

**Honest scope:** running 3 copies on one VPS isn't *geographically* distributed, but it correctly demonstrates **distributed coordination** — independent processes, a shared queue, no double-processing, load balancing. Phrase it that way; don't overclaim a "global cluster".

## Fault tolerance (the reliability story)

- **Worker crash recovery:** if a worker goroutine dies, the pool detects it and respawns a replacement; the in-flight task is requeued (not lost).
- **Hung worker watchdog:** a watchdog checks periodically (every ~15s) and kills/requeues tasks stuck longer than a timeout (~60s).
- **Orphan recovery on startup:** if the whole backend restarts, tasks left in `running` (orphaned) are requeued via `requeueTask()` — which does **not** increment `retry_count` (the task didn't fail, the process died).
- **Retries + Dead Letter Queue:** a failing task retries up to `max_retries`; after that it moves to `dead_letter_queue` for inspection/manual retry.

## What this proves about you (the resume subtext)

- You understand **concurrency** (worker pools, goroutines, channels).
- You understand **coordination** (shared queue, locking, no double-processing).
- You understand **reliability** (crash recovery, retries, DLQ, graceful degradation).
- You understand **tradeoffs** (why DB queue not Kafka, why SSE not WebSockets).
- You can **measure** your system (load tests → real numbers).

→ Next: [02 — Database](02-DATABASE.md)
