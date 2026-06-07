# Interview Notes — Problems Solved & How

> **Living document.** Every phase adds the real problems we hit and how we solved them —
> framed so I can *talk* about them. This is the proof we genuinely engineered this thing,
> not just wired libraries together. Each entry: **Problem → Solution → "the line you say"**,
> plus likely follow-up questions for the meaty ones.
>
> Status: covers Phases 0–6 (backend complete). Append new sections as later phases land.

---

## The one-sentence pitch

> "A from-scratch distributed background job processing engine in Go: clients submit tasks
> over an API, the tasks live in a MySQL table that *is* the queue, a pool of worker
> goroutines claims and runs them concurrently with `SELECT ... FOR UPDATE SKIP LOCKED`,
> failures retry and then dead-letter, and a React dashboard watches it all live over SSE."

---

# Phase 6 — Real-time + analytics

## A. Rate limiting — "how do you stop one client from hammering the API?"

**Problem:** Any client could flood the API with requests, starving others and overloading the DB. I
need a per-client cap — e.g. 10 requests / 60s — with the (N+1)th getting `429 Too Many Requests`.

**Solution:** A **sliding-window** rate limiter, per client, backed by **Redis**, run as Gin
middleware *after* auth (so I limit by `client_id`, not IP — one tenant's burst can't eat another's
allowance).

Why sliding window over a fixed-window counter: a fixed per-minute counter lets a client send 10 at
`12:00:59` and 10 more at `12:01:00` — 20 in one real second, because they straddle the bucket edge.
A sliding window always looks at "the last 60 seconds from *now*", so there's no boundary to exploit.

Implementation — a **sorted set per client** (`ratelimit:<client_id>`), request timestamps as scores:
1. `ZREMRANGEBYSCORE` — drop entries older than `now - window` (slide the window),
2. `ZCARD` — count what's left,
3. if under the limit: `ZADD` this request + `PEXPIRE` the key; else deny.

The catch: those three steps must be **atomic**, or two simultaneous requests could both read
"count = max-1" and both get admitted. So I run them as a single **Lua script** inside Redis — Lua
scripts execute to completion with nothing interleaved. That's the check-then-act lesson again (same
as the dequeue and the cancel race): push the decision to where it can be made atomically.

On a Redis error the middleware **fails open** (allows the request) — a limiter outage shouldn't take
the whole API down. (Planned follow-up: an in-memory fallback limiter instead of fully open.)

**The line you say:** *"Per-client sliding-window limiter in Redis using a sorted set of request
timestamps, and the prune-count-admit logic runs as one atomic Lua script so concurrent requests
can't both slip past the limit. It's keyed by client_id so it's fair per tenant, and it fails open if
Redis is down."*

**Follow-ups:**
- *Why Redis, not in-memory?* → "So the limit is shared across all backend instances — with 3
  instances behind a load balancer, an in-memory counter would let a client do 3x the limit. Redis is
  the shared source of truth. I'd add an in-memory fallback only for when Redis is unreachable."
- *Sliding window log vs sliding window counter?* → "I used the log (one entry per request) — exact,
  at the cost of memory proportional to the limit. The counter variant (weighting current + previous
  fixed window) uses O(1) memory and is the move at very high request volumes."
- *Why fail open, not closed?* → "Availability call: blocking all traffic because the *limiter* is
  down is usually worse than briefly not limiting. For an abuse-sensitive endpoint you might fail closed."

## B. Analytics — "how do you report on the system's behaviour?"

**Problem:** The dashboard needs metrics: how many tasks in each state, how often they fail, how long
they take to run, how long they wait in the queue, and recent throughput — all per client.

**Solution:** A read-only `AnalyticsService` (separate from `TaskService` — different job: it only
aggregates, never mutates) behind `GET /api/analytics`, client-scoped like everything else. It's
straight SQL aggregation:
- **status breakdown + total:** one `GROUP BY status` pass.
- **failure rate:** `failed / (completed + failed)` — only *finished* work counts toward success/fail.
- **avg execution time:** `AVG(completed_at − started_at)` over completed tasks.
- **avg queue wait:** `AVG(started_at − created_at)` — the time before a worker picked it up.
- **throughput:** count completed in the last hour.

Two details worth mentioning: I used MySQL's `TIMESTAMPDIFF(MICROSECOND, a, b)/1000` for millisecond
durations, and wrapped the average in a `sql.NullFloat64` because `AVG` over zero rows returns `NULL`,
not 0 — so a brand-new client doesn't blow up the query.

**The line you say:** *"A separate read-only analytics service does GROUP BY / AVG aggregation per
client — status counts, failure rate, execution time, queue wait, throughput. I split reporting from
the task service because reads and mutations are different responsibilities, and the queue table is
already the source of truth, so no separate analytics store is needed at this scale."*

**Follow-ups:**
- *Won't these aggregates get slow as the table grows?* → "Yes — `GROUP BY status` and time-range
  scans get expensive at millions of rows. Next steps: indexes on `(client_id, status)` and
  `completed_at`, then pre-aggregated rollups (a periodic job writing per-minute summaries) so the
  dashboard reads small tables instead of scanning the whole queue."
- *Why not compute these in Go?* → "The DB aggregates far more efficiently than pulling every row into
  the app and looping. Push computation to the data."

## C. Real-time updates with SSE — "how does the dashboard update live?"

**Problem:** The dashboard should reflect task state the instant it changes, without the browser
polling the API every second (wasteful, laggy).

**Solution:** **Server-Sent Events (SSE)** — the browser opens one long-lived HTTP connection
(`GET /api/sse/events`) and the server *pushes* events down it. I chose SSE over WebSockets because
the data flow is one-directional (server → browser) and SSE is just HTTP — no separate protocol,
auto-reconnect built into the browser's `EventSource`.

Architecture:
- A **Hub** keeps a registry of connected dashboards (each a buffered channel). The worker pool, on
  every task `started`/`completed`/`failed`, calls `PublishTaskEvent`; the hub fans it out.
- **Per-client filtering:** the hub only delivers an event to subscribers whose `client_id` matches
  the task's owner — same multi-tenant isolation as the rest of the API. Alpha never sees beta's events.
- **Decoupling:** the pool depends on a tiny `Publisher` interface (`PublishTaskEvent(...)`), so it
  doesn't import the sse package at all — same trick as the scheduler's `Dispatcher`.
- **Non-blocking publish:** sends to a subscriber use `select { case ch <- e: default: }` — if a slow
  dashboard's buffer is full, we *drop* the update rather than block the worker goroutine. A missed UI
  refresh is acceptable; a stalled worker is not.
- **Heartbeat:** the handler sends a heartbeat event every 15s so the connection survives proxies and
  we detect dead clients; disconnect is caught via the request's context being cancelled.

**The line you say:** *"Live updates over SSE: a hub holds open connections and the worker pool
publishes lifecycle events to it, fanned out per client_id for tenant isolation. Publishing is
non-blocking so a slow browser can't stall a worker, and the pool only knows a small Publisher
interface, not the SSE package. SSE over WebSockets because the flow is one-way and it's plain HTTP."*

**Follow-ups:**
- *SSE vs WebSocket vs polling?* → "Polling wastes requests and adds latency. WebSockets are
  bidirectional and heavier. My traffic is server→client only, so SSE — one HTTP connection, browser
  auto-reconnect, simplest thing that works."
- *What happens to events while a client is disconnected?* → "They're dropped — SSE is fire-and-forget
  here. The dashboard fetches a fresh snapshot (the list/analytics endpoints) on (re)connect, then
  SSE keeps it live. For guaranteed delivery you'd need an event log / Last-Event-ID replay."
- *Does this work with multiple backend instances?* → "Each instance only has the connections and
  events for the tasks *it* runs. Behind a load balancer a browser connects to one instance and sees
  that instance's events. Full cross-instance fan-out would need a Redis Pub/Sub backplane — a clear
  next step."

---

# Phase 5 — Fault tolerance

## A. Graceful shutdown — "what happens to running work when you deploy/restart?"

**Problem:** The server originally ended in `r.Run()`, which blocks forever. On Ctrl+C / SIGTERM
(a deploy, a container restart) the OS **kills the process instantly** — every task mid-execution is
abandoned, left stuck in `running` (an orphan), and all my cleanup `defer`s are skipped because
`main()` never returns. That's lost work on every single restart.

**Solution:** Catch the signal and shut down in a deliberate order instead of dying:
1. Swapped `r.Run()` for a standard-library `http.Server` (it has a `Shutdown()` method; `r.Run()`
   gives no handle to stop it). Run it in a goroutine so `main` can move on.
2. `main` blocks on a channel fed by `signal.Notify(SIGINT, SIGTERM)`. The signal now lands in *my*
   code instead of killing the process.
3. On signal, shut down in order: **stop the HTTP server** (refuse new requests) → **stop the
   scheduler** (stop handing out work) → **drain the worker pool** (let in-flight tasks *finish*) →
   close Redis, close MySQL. That order is enforced by the **LIFO order of the `defer`s** in `main`.

The subtle part is the **drain vs. abort** distinction in the pool. My first instinct (and the
original Phase 3 code) cancelled a `context` on shutdown — but the handlers *watch* that context, so
that would **cut tasks off mid-run** (they'd return an error and get wrongly counted as a retry). A
true graceful stop **closes the task channel** instead; the workers `range` over it, so they finish
everything already buffered or in-flight and *then* the loop ends. Closing the channel is safe only
because the scheduler is guaranteed stopped first (the defer order), so nothing can `Submit` after
the close and panic.

**The line you say:** *"On SIGTERM I stop intake first, then drain in-flight work before closing —
and the key detail is I drain by closing the work channel and letting workers finish, not by
cancelling their context, so no in-flight task is cut off or wrongly retried. Zero loss on restart."*

**Follow-ups:**
- *How do you know it drained and didn't abort?* → "In the logs you see `task started`/`task completed`
  lines printing *after* `shutdown signal received` — those are buffered/in-flight tasks finishing.
  And `SELECT COUNT(*) WHERE status='running'` is 0 after exit: nothing left stranded."
- *What if a task hangs forever during drain?* → "Right now drain waits on the WaitGroup. The
  production hardening is a bounded drain — wait up to N seconds, then hard-cancel the stragglers
  (which become orphans recovered on next boot). That pairs with the hung-worker watchdog."
- *Why does ordering matter?* → "If I closed the pool before stopping the scheduler, the scheduler
  could `Submit` to a closed channel and panic. Stop producers before consumers — always."

## B. Durability — "a client submits a task, then the server crashes a second later. Lost?"

**(Great 'do you understand durability' question — the answer is a crisp rule.)**

**Problem:** Where exactly does a task become *safe*? If the server dies right after a client hits
submit, does the task survive?

**Solution / the mental model:** It depends on **whether the server acknowledged it**:
- If the POST returned **`201 Created`**, the task was already **written to the MySQL `tasks` table as
  `pending`** before I replied. The queue *is* that table, and it's on disk — **durable**. A crash/
  restart just re-reads the table: `pending` tasks get scheduled, `running` tasks get orphan-recovered.
  **Not lost.**
- If the request **never got a `201`** (e.g. connection refused because the server was down), it was
  **never written anywhere** — there's nothing to lose and nothing to recover. By the at-least-once
  contract, **retrying is the client's responsibility.**

**The line you say:** *"Acknowledgement is the durability boundary. Once I return 201 the task is in
MySQL, so it survives any crash — pending tasks just run on restart, running tasks are orphan-
recovered. Anything that never got a 201 was never accepted; the client retries. That's the
at-least-once contract."*

**Follow-up:**
- *So could a task run twice?* → "Yes — at-least-once means a task acked but crashed mid-run gets
  retried, so handlers should be idempotent. I chose at-least-once (never lose work) over at-most-once
  (never duplicate); for a job queue, losing work is the worse failure."

## C. Debugging story — "`go run` made graceful shutdown *look* broken"

**Symptom:** On Ctrl+C the shell prompt appeared *in the middle* of the shutdown logs, so it looked
like the server hung or didn't stop.

**Cause:** `go run` is **two** processes — the `go run` wrapper *and* the compiled child binary. Ctrl+C
goes to the whole foreground process group, so the wrapper quits immediately (the shell prints a fresh
prompt) while the *real* server is still alive finishing its drain and logging to the same terminal.

**Fix:** Run the built binary directly (`go build -o task-engine . && ./task-engine`) — one process,
so Ctrl+C reaches the server, it drains, exits, and the prompt returns cleanly *after* all output.

**The takeaway line:** *"It wasn't a shutdown bug — it was a `go run` signal-forwarding artifact. Worth
knowing because in prod you ship the binary, not `go run`, so the real behaviour is the clean one."*

## D. Panic recovery — "one task hits a bug and panics. What happens to the rest?"

**(A Go-specific gotcha most people get wrong — strong signal you actually know the language.)**

**Problem:** Each task runs inside a worker goroutine. In Go, an **unrecovered panic in *any*
goroutine crashes the *entire* process** — not just that goroutine. So a single buggy task (nil
dereference, bad type assertion, divide-by-zero) would take down the HTTP server and *all* the
workers with it. One poisoned job = total outage.

**Solution:** I wrap every handler call in a `recover()` safety net (`runHandler`). If the handler
panics, I recover it, log it **with a full stack trace** (`debug.Stack()`), and convert the panic
into a normal `error`. That error then flows down the existing **fail → retry → dead-letter** path,
and crucially the **worker survives** and pulls the next task. A panic becomes "that one task
failed," not "the engine died."

```go
func runHandler(ctx context.Context, handler Handler, task *models.Task) (err error) {
    defer func() {
        if r := recover(); r != nil {
            slog.Error("recovered from handler panic", "task_id", task.ID, "panic", r,
                "stack", string(debug.Stack()))
            err = fmt.Errorf("handler panicked: %v", r)
        }
    }()
    return handler(ctx, task)
}
```

The detail that makes it work: the **named return value `(err error)`** — a deferred function can
only change the function's result if the return is named, so that's how the recovered panic gets
handed back as an error.

**How I proved it:** added a chaos task type (`{"type":"panic"}`) that deliberately panics. Submitted
one with `max_retries:2`: it panicked, got recovered and retried three times, then dead-lettered —
and a normal task submitted right after **completed**, proving the pool lived through three panics.

**The line you say:** *"A panic in a goroutine crashes the whole Go process, so every task runs behind
a `recover()`. A panicking task is caught, logged with a stack trace, and routed through the normal
retry/dead-letter path — the worker and the process stay up. I verified it with a chaos task type."*

**Follow-ups:**
- *Should a panic be retried like a normal failure?* → "I route it through retries, but a panic is
  usually a deterministic bug, so it'll just re-panic until it dead-letters — which is fine: bounded
  retries stop the loop and the DLQ captures it for inspection. You could also fast-path panics
  straight to the DLQ."
- *Where exactly is the recover?* → "Around the handler call only — not around my own
  complete/fail bookkeeping. I want to catch *user task code* panics, not mask bugs in the engine."

## E. The watchdog + the three-legged recovery story — "how do you guarantee no task is lost?"

**(The capstone fault-tolerance answer — ties the whole phase together.)**

**Problem:** A task in `running` means a worker claimed it. But the worker/instance can die or hang
*after* claiming and *before* finishing, leaving the task frozen in `running` forever — an **orphan**.
There are three distinct ways this happens, and one mechanism can't cover all three.

**Solution — three complementary mechanisms, each covering what the others miss:**

| Failure | Mechanism | Notes |
|---|---|---|
| This instance **crashed**, now rebooting | **Startup recovery** (`RequeueOrphanedTasks`) | Runs once at boot. Scoped to *this* instance's id. **No** retry bump (the instance died, not the task). |
| Clean **shutdown** (deploy/restart) | **Graceful drain** | Finish in-flight work before exiting (section A). |
| A **live** instance with a **hung** task, or a **dead peer** that never came back | **Watchdog** | Timer loop: find tasks `running` past a timeout, requeue them. **Not** instance-scoped, so it rescues dead peers too. |

The **watchdog** is the new piece: every `interval` (15s) it runs `SELECT ... WHERE status='running'
AND started_at < now - timeout` (60s) and routes each stuck task through `FailTask`.

**Two design decisions I can defend:**
1. **Timeout = failure (bump retries).** Unlike startup recovery, the watchdog *does* count it as an
   attempt. Why: a task that hangs *every* time would otherwise be requeued forever (a livelock).
   Treating the timeout as a failure means bounded retries → it eventually dead-letters. The timeout
   is set well above the longest real task so a merely-slow task is never wrongly reclaimed.
2. **Not instance-scoped.** Startup recovery only fixes *your own* boot; if a peer instance dies and
   never restarts, only *another* instance's watchdog can rescue its tasks. So the watchdog
   deliberately looks at *all* stale `running` tasks, not just its own.

**The line you say:** *"No single mechanism is enough, so I have three: startup recovery for my own
crash, graceful drain for clean shutdowns, and a watchdog timer for hung workers and dead peers.
Together they guarantee at-least-once — every accepted task eventually completes or dead-letters."*

**Follow-ups:**
- *Doesn't the watchdog risk running a task twice?* → "Yes — if a 'stuck' task is actually a slow-but-
  alive worker, requeuing it could double-run it. That's the at-least-once trade-off: I'd rather run
  twice than lose work, so handlers should be idempotent. The big timeout makes a false positive rare."
- *Why a polling timer instead of heartbeats?* → "Heartbeats (workers update a `last_heartbeat`
  column) detect death faster and more precisely, and that's the natural next step. Timeout-polling is
  simpler and good enough when task durations are bounded and known."
- *How did you prove it?* → "I `UPDATE`d a task to `running` with `started_at` 5 minutes ago and
  `processed_by='dead-instance-99'`. Within one interval the watchdog logged `recovered stuck task`,
  requeued it, and another worker completed it — cross-instance crash recovery, demonstrated."

---

# Phase 4 — The scheduler (fairness / anti-starvation)

## 0. Multi-tenant fairness — "one client floods the queue; how do the others not starve?"

**(The distributed-systems story — shows I think about *fairness*, not just correctness.)**

**Problem:** In Phase 3 the workers just claimed "the globally-next pending task" by priority. That's
correct, but unfair: if Client A dumps 1,000 tasks and Client B submits 5 right after, B's tasks sit
behind all 1,000 of A's. One noisy tenant starves everyone else — a classic multi-tenant problem.

**Solution:** I split the decision in two and added a **scheduler** layer that runs **Deficit Round
Robin (DRR)**:
- **Fairness is decided in Go** — *which client* goes next. On a 250ms timer the scheduler cycles
  through every client that has pending work and lets each dispatch up to a **quantum** of tasks per
  round. So A and B take turns; A's flood can't jump ahead of B.
- **Priority is still decided in SQL** — *which task within that client*. The per-client dequeue is
  still `ORDER BY priority DESC, created_at ASC ... FOR UPDATE SKIP LOCKED`, scoped
  `WHERE client_id = ?`.

The "deficit" part: each client carries a `deficit` counter. Each round I add `quantum` to it and
dispatch while it's positive. Leftover credit carries to the next round — that's what makes it fair
when tasks have *different costs* (a client that could only afford a big job this round gets to use
the carried credit next round). A client whose queue empties has its deficit reset to 0 so it can't
hoard credit and jump the line when it comes back.

**The line you say:** *"I separated fairness from priority. A DRR scheduler in Go decides which
client dispatches next so no tenant starves; the database still decides which task within that client
by priority. Fairness in Go, priority in SQL."*

**Follow-ups:**
- *Isn't this just round-robin?* → "With uniform task costs, yes — DRR degenerates to weighted
  round-robin. The deficit mechanism earns its keep when tasks have heterogeneous costs: the carried
  deficit lets a client 'save up' for work it couldn't afford in one round, which plain RR can't do."
- *Why a timer instead of event-driven?* → "Simplicity and natural batching — every 250ms it does one
  fair pass. The backpressure from the worker pool (a full channel blocks `Submit`) paces it so it
  never over-dispatches."
- *What stops two timer ticks from running passes at once?* → "An `isScheduling` mutex-guarded flag.
  Each tick fires its pass in its own goroutine, so if a pass is slow (blocked on a full worker
  queue), the next tick sees the flag and bows out. Exactly one pass runs at a time, so the
  shared `deficits` map needs no further locking."
- *Why decouple scheduler from the pool?* → "The scheduler depends on a tiny `Dispatcher` interface
  (`Submit(task) bool`), not the concrete pool — so the two halves stay independent and testable."

## 0b. Startup recovery of orphans — "a box dies mid-job; what happens to that task?"

**Problem:** A task in `status='running'` means a worker claimed it. But if the *instance* crashes or
restarts mid-job, the row is frozen at `running` forever — no live worker is actually running it. It's
an **orphan**: claimed on paper, abandoned in reality. It would never complete and never retry.

**Solution:** On startup, *before* the scheduler hands out any work, the instance requeues **its own**
orphans — flips `running` rows back to `pending` and clears the timing/owner fields:
```sql
UPDATE tasks SET status='pending', started_at=NULL, processed_by=NULL
WHERE status='running' AND processed_by = <this-instance-id>;
```
Two deliberate choices:
- **Scoped to `processed_by = me`** — I only reclaim tasks *this* instance abandoned. Other instances'
  `running` tasks might genuinely be in progress; stealing them would double-execute.
- **No `retry_count++`** — the task didn't *fail*, the *process* died. Punishing the task's retry
  budget for an infrastructure crash would be wrong, so it goes back to `pending` as good as new.

**The line you say:** *"On boot I requeue orphaned `running` tasks back to `pending` — scoped to my
own instance id so I don't steal live work, and without bumping the retry count because the task
didn't fail, the instance did."*

**Follow-up:**
- *What about a worker that hangs but the instance is alive?* → "That needs a watchdog with a
  heartbeat/timeout to detect stuck `running` tasks — Phase 5."

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

- **Phase 6 — Real-time + analytics:** SSE live updates, analytics queries, Redis sliding-window
  rate limiter.
- **Phase 6+ — Rate limiting (Redis sliding window), SSE live updates, analytics queries, Docker
  Compose + nginx load balancing (the "distributed" proof: multiple backend instances sharing the
  queue via `processed_by`).**
