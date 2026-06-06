# Interview Notes — Problems Solved & How

> **Living document.** Every phase adds the real problems we hit and how we solved them —
> framed so I can *talk* about them. This is the proof we genuinely engineered this thing,
> not just wired libraries together. Each entry: **Problem → Solution → "the line you say"**,
> plus likely follow-up questions for the meaty ones.
>
> Status: covers Phases 0–3. Append new sections as later phases land.

---

## The one-sentence pitch

> "A from-scratch distributed background job processing engine in Go: clients submit tasks
> over an API, the tasks live in a MySQL table that *is* the queue, a pool of worker
> goroutines claims and runs them concurrently with `SELECT ... FOR UPDATE SKIP LOCKED`,
> failures retry and then dead-letter, and a React dashboard watches it all live over SSE."

---

# Phase 3 — Concurrency (the heart of the project)

## 1. The dequeue race — "how do you stop two workers running the same job?"

**Problem:** The queue is just a MySQL table. I run a pool of worker goroutines (and eventually
multiple backend instances). If two workers both `SELECT` the next pending task, they'd both grab
the *same* row and run the job twice.

**Solution:** I claim tasks with `SELECT ... FOR UPDATE SKIP LOCKED` inside a transaction.
`FOR UPDATE` locks the row I'm claiming; `SKIP LOCKED` makes other workers *step over*
already-locked rows instead of blocking on them. Each worker grabs a *different* task — no
double-processing, no lock contention.

```sql
SELECT id, ... FROM tasks
WHERE status = 'pending'
ORDER BY priority DESC, created_at ASC
LIMIT 1
FOR UPDATE SKIP LOCKED;
-- then, same transaction: UPDATE that row to status='running', stamp processed_by
```

**The line you say:** *"I used the database as the synchronization point. `SELECT FOR UPDATE
SKIP LOCKED` lets N workers pull distinct jobs concurrently with no locking contention and no
duplicate execution — the same pattern DB-backed queues like Que use."*

**Follow-ups:**
- *Why not Redis/Kafka?* → "I wanted transactional claiming against the same DB that stores the
  tasks, without adding a broker. For this scale it's simpler and the DB is already the source of truth."
- *What if a worker crashes mid-job?* → "The row stays `running`; startup-recovery + a watchdog
  requeue stuck `running` tasks." (Phase 5)
- *Why `ORDER BY priority DESC, created_at ASC`?* → "Highest priority first, oldest-first within a
  priority (FIFO). Backed by a composite index `(status, priority, created_at)`."

## 2. The worker pool design — "walk me through your concurrency model"

**Problem:** I need controlled parallelism — run many jobs at once, but cap the load so I don't
exhaust DB connections.

**Solution:** One **dispatcher goroutine** is the only thing that claims tasks (claim logic in one
place); it pushes them onto a **buffered channel**. **N worker goroutines** read from the channel
and run jobs. N = concurrency level, from config. Clean shutdown via `context.Context`
(cancels in-flight work) + `sync.WaitGroup` (wait for all workers to exit).

**The line you say:** *"It's a classic fan-out: one producer claiming work, a bounded channel for
backpressure, a fixed pool of consumers. The channel buffer plus fixed worker count caps
concurrency; context + WaitGroup give graceful shutdown."*

**Follow-ups:**
- *Why a single dispatcher, not let each worker claim?* → "Simplicity — one place owns the claim
  logic. At higher throughput I'd let each worker claim its own task to remove the dispatcher as a
  bottleneck; `SKIP LOCKED` already makes that safe."
- *What's backpressure here?* → "If all workers are busy, the dispatcher blocks on the full channel
  and stops claiming — I never pull more work than I can run."

## 3. The cancel race (TOCTOU) — "tell me about a concurrency bug you found and fixed"

**(Strongest story — a bug I spotted and reasoned through, not just a feature.)**

**Problem:** My cancel endpoint did read-then-write: `SELECT` to check the task was
`pending`/`running`, then `UPDATE` to set `cancelled`. With no workers that was fine. Once I added
the worker pool, there was a gap: a worker could claim the task and flip it to `running` *between*
my check and my write, and my write would blindly overwrite it. A classic
**time-of-check-to-time-of-use** race.

**Solution:** Collapsed it into a single atomic guarded update — moved the status check *into* the
`UPDATE`:
```sql
UPDATE tasks SET status='cancelled', completed_at=NOW()
WHERE id=? AND client_id=? AND status IN ('pending','running');
```
The DB checks and writes as one locked operation — no gap. Then I read `RowsAffected`: `1` =
cancelled; `0` = guard rejected it, and a quick follow-up read decides 404 (not found) vs 409
(already terminal).

**The line you say:** *"I had a check-then-act race in cancel. The fix was to stop making the
decision in application code and push it into the database — a conditional `UPDATE` guarded by a
`WHERE` on current status, so check and write are atomic. Same principle as the dequeue: let the DB
be the referee for who touches the row."*

**Follow-ups:**
- *How did you detect it?* → "Reasoned about it when adding the pool — the gap was theoretically
  there in Phase 2 but harmless because nothing else touched rows; adding workers made it real."
- *Cancelling a job already mid-execution?* → "Marks intent; truly stopping the work needs
  cooperative cancellation (the handler watching a context) — a deeper feature."

## 4. Retries + dead-letter queue — "how do you handle failures?"

**Problem:** Jobs fail (transient errors). I want automatic retries but not infinite loops.

**Solution:** On failure, if `retry_count < max_retries` I put the task back to `pending`, bump the
count, and **clear the timing fields** so the next attempt is clean. When retries are exhausted I
**dead-letter** it: in one transaction I mark the original row `failed` *and* copy it into a
`dead_letter_queue` table. The transaction guarantees I never get one without the other.

**The line you say:** *"Bounded retries via a counter, then a dead-letter queue for terminal
failures — and the dead-letter move is transactional so the two writes can't get out of sync."*

---

# Phase 2 — API & multi-tenancy

## 5. Multi-tenant isolation — "how do you keep one client from seeing another's data?"

**Problem:** Multiple clients share one database. Client A must never read/cancel Client B's tasks.

**Solution:** API-key auth resolves the caller to a `client_id` (stored in request context). *Every*
query is scoped `WHERE client_id = ?`. A cross-tenant fetch returns **404, not 403** — I don't even
confirm the row exists to someone who doesn't own it.

**The line you say:** *"Authorization is enforced by scoping every query to the authenticated
client_id, and I return 404 on cross-tenant access so I don't leak that the resource exists."*

## 6. SQL injection defense — "how do you build a flexible query safely?"

**Problem:** The list endpoint accepts filters, sorting, and pagination from user input. Naively
interpolating those into SQL is an injection hole.

**Solution:** Two different defenses for two different things:
- **Values** (status, type, priority) use parameterized `?` placeholders — the driver escapes them.
- **The sort column** *can't* be a `?` placeholder (it's SQL structure, not a value), so I
  **whitelist** it against a fixed allow-list map; anything not on the list falls back to the
  default. Order is forced to `asc`/`desc`.

**The line you say:** *"Parameterized queries for values, but a sort column can't be parameterized —
it's part of the SQL text — so that one I whitelist. Two different tools because they're two
different classes of input."*

## 7. Layered architecture + DI — "how is the code organized?"

**Problem:** Keep HTTP concerns, business logic, and data access from bleeding into each other so the
logic is testable and reusable (the worker pool reuses the *same* service the API uses).

**Solution:** `routes → controllers → services → models`. Controllers only do HTTP
(parse/validate/respond); services hold business logic and own the DB; dependencies are injected at
startup. Result: the worker pool and the HTTP layer share **one** `TaskService` instance — same
rules everywhere.

**The line you say:** *"Clean layering with dependency injection. The business logic doesn't know
about Gin or HTTP, so the worker pool calls the exact same service methods the API does."*

---

# Phase 0–1 — Foundation

## 8. Things that show production-mindedness (mention in passing)

- **Structured logging** (`slog` JSON) — machine-parseable logs, ready for aggregation.
- **Config from environment** with sane defaults — no hardcoded secrets; `.env` in dev, injected
  in prod.
- **DB connection pooling** tuned explicitly, plus a startup **ping** so the app fails fast if the
  DB is unreachable.
- **Idempotent migrate + seed on startup** — the app brings its own schema up (GORM AutoMigrate)
  and seeds sample data only when empty.
- **Centralized error handling** — services return typed sentinel errors; one place maps them to
  HTTP status codes, so controllers stay thin.

---

# The meta-point (say this for "what did you learn?")

> "The recurring lesson was: when multiple actors touch the same row, don't make the decision in
> application code and hope nothing changes before you write. Push the decision into the database as
> an atomic conditional operation — `SKIP LOCKED` for claiming, guarded `UPDATE`s for state
> transitions. The DB is the single source of truth *and* the synchronization point."

---

# To add in later phases

- **Phase 4 — Scheduler (Deficit Round Robin):** fair scheduling so one client can't starve others.
- **Phase 5 — Fault tolerance:** startup recovery of orphaned `running` tasks, hung-worker watchdog,
  graceful shutdown on SIGTERM.
- **Phase 6+ — Rate limiting (Redis sliding window), SSE live updates, analytics queries, Docker
  Compose + nginx load balancing (the "distributed" proof: multiple backend instances sharing the
  queue via `processed_by`).**
