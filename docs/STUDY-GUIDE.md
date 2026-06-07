# Study Guide — everything to revise before interviews

> One place to prep. Each topic is tied to **what you actually built**, so you're never studying in the
> abstract — you can answer with "in my project, I…". Pairs with `INTERVIEW-NOTES.md` (the
> problem→solution war stories) — this file is the **concepts to study around them**.
>
> **How to use it:** for each topic, be able to (a) explain it in plain words, (b) point to where it
> shows up in your project, (c) handle the "what if…" follow-up. ⭐ = highest priority.

---

## 1. Queues & background job processing ⭐

**You built:** a job queue where the **MySQL table IS the queue** (`pending → running → completed/failed`).

**Study:**
- DB-backed queue vs a dedicated broker (**Kafka / RabbitMQ / SQS / Redis lists**) — trade-offs.
- **At-least-once vs at-most-once vs exactly-once** delivery (you chose at-least-once → handlers must be **idempotent**).
- **Dead-letter queues**, retries with **exponential backoff + jitter**, **visibility timeout**.
- Pull vs push workers; **fan-out** (one producer → many consumers).

**Likely Qs:** "Why a DB and not Kafka?" · "How do you avoid running a job twice?" · "What happens to a job that keeps failing?"

---

## 2. Concurrency (Go) ⭐

**You built:** N worker goroutines pulling from a buffered channel; graceful shutdown; panic recovery.

**Study:**
- **Goroutines vs threads** (cheap, scheduled by the Go runtime); the **worker-pool** pattern.
- **Channels** (buffered vs unbuffered), `select`, `context.Context` cancellation, `sync.WaitGroup`, `sync.Mutex`.
- **Back-pressure** via a bounded channel (your buffer caps concurrency).
- **Race conditions**, the Go race detector (`-race`), why a panic in *any* goroutine crashes the whole process (→ your `recover()`).
- Graceful shutdown: catch SIGTERM → stop intake → **drain** in-flight → close.

**Likely Qs:** "Walk me through your concurrency model." · "How do you shut down without losing work?" · "What's back-pressure?"

---

## 3. The database as the synchronization point ⭐ (your crown jewel)

**You built:** `SELECT ... FOR UPDATE SKIP LOCKED` to claim jobs; atomic guarded `UPDATE`s for cancel.

**Study:**
- **Row locking**: `FOR UPDATE`, **`SKIP LOCKED`** (skip locked rows instead of blocking), `NOWAIT`.
- **Transactions & ACID**, **isolation levels** (read-committed, repeatable-read, serializable), what each prevents (dirty/non-repeatable/phantom reads).
- **TOCTOU / check-then-act races** → fix by pushing the decision into one atomic statement (your cancel bug).
- **Indexes**: your composite `(status, priority, created_at)` dequeue index; B-trees; covering indexes; `EXPLAIN`.
- Optimistic vs pessimistic locking.

**Likely Qs:** "How do two workers not grab the same row?" · "Walk me through SKIP LOCKED." · "What isolation level and why?"

---

## 4. Fault tolerance & reliability ⭐

**You built:** three-legged recovery (startup orphan-requeue, graceful drain, hung-worker watchdog) + panic recovery.

**Study:**
- **Crash recovery**: orphaned `running` rows; heartbeats/timeouts to detect dead/hung workers.
- **Idempotency** (so retries are safe), **at-least-once** consequences.
- **Timeouts everywhere**, retries + backoff, **circuit breakers**, **bulkheads**.
- Liveness vs readiness; **graceful degradation**; failing **open vs closed**.

**Likely Qs:** "A worker dies mid-task — what happens?" · "How do you detect a stuck job?" · "Why no double-execution after a crash?"

---

## 5. Scheduling & fairness

**You built:** Deficit Round Robin (DRR) so one tenant can't starve others; "fairness in Go, priority in SQL".

**Study:**
- **Round-robin, weighted RR, DRR, fair queuing**; priority queues; **starvation**.
- Multi-tenant fairness; noisy-neighbor problem.

**Likely Qs:** "One client floods the queue — how do others not starve?" · "Where's priority decided vs fairness?"

---

## 6. Rate limiting ⭐

**You built:** cost-weighted sliding-window limiter in Redis on the submit path (see `INTERVIEW-NOTES.md` A/A2).

**Study:** fixed window, **sliding-window log/counter**, **token bucket**, leaky bucket; atomicity (Lua); edge vs app-level; `429`/`Retry-After`; weighted/cost-based limiting.

**Likely Qs:** "Design a rate limiter." · "Why weight by task count?" · "Where would you put it in a distributed system?"

---

## 7. Distributed systems ⭐

**You built:** N stateless instances sharing MySQL+Redis; Redis worker registry with heartbeat+TTL; `SKIP LOCKED` for coordination.

**Study:**
- **Horizontal scaling**, **stateless services**, **shared state** (DB/Redis) vs peer-to-peer.
- **Heartbeats + TTL** for liveness; **service discovery**; **load balancing** (round-robin, least-conn).
- **CAP theorem**, consistency models (strong vs eventual); **leader election** (where you'd need it and why you *don't* here — the DB is the coordinator).
- **Idempotency keys**, distributed locks (and their dangers).

**Likely Qs:** "How do you run this on 3 machines?" · "How does the dashboard see all instances?" · "What needs to be global vs per-instance?"

---

## 8. Caching & Redis ⭐

**You built:** Redis for the worker registry + rate limiter (shared, ephemeral, TTL-based).

**Study:**
- Redis data types (strings, **sorted sets**, hashes, lists), **TTL/expiry**, `EVAL`/Lua, `KEYS` vs `SCAN`, pub/sub.
- Redis vs a DB: **in-memory + ephemeral** (great for coordination) vs durable source of truth.
- Cache strategies (cache-aside, write-through), eviction (LRU), **cache invalidation**, thundering herd.

**Likely Qs:** "Why Redis and not MySQL for the registry?" · "How do dead instances disappear?" · "What's a TTL good for?"

---

## 9. API design, multi-tenancy & security

**You built:** REST API, API-key auth → `client_id`, per-tenant scoping (404 cross-tenant), SQL-injection defenses, pagination, batch endpoint.

**Study:**
- **REST** semantics, status codes (200/201/400/401/404/409/429), idempotency, pagination (offset vs **cursor/keyset**).
- **AuthN vs AuthZ**; multi-tenant **isolation** (scope every query by tenant); why 404 not 403.
- **SQL injection** (parameterized queries; whitelisting columns you can't parameterize — your sort whitelist).
- **Batch endpoints** (bulk insert vs N round-trips), input validation.

**Likely Qs:** "How do you keep tenant A from seeing tenant B's data?" · "How do you build a safe dynamic filter/sort?" · "Offset vs cursor pagination?"

---

## 10. Real-time updates

**You built:** SSE hub pushing live task events; `fetch-event-source` (headers!); per-client fan-out; heartbeats.

**Study:**
- **SSE vs WebSockets vs long-polling vs short-polling** — when each fits (you picked SSE: one-way, plain HTTP, auto-reconnect).
- Connection management, **heartbeats**, backpressure on slow clients (you drop, non-blocking).
- Cross-instance fan-out → **Redis pub/sub** backplane (the next step you can name).

**Likely Qs:** "Why SSE over WebSockets?" · "What if a client is slow?" · "How would this work with 3 backends?"

---

## 11. Observability

**You built:** structured `slog` JSON logging (+ text dev mode), analytics endpoints, per-task audit trail.

**Study:**
- **Structured logging**, log levels, correlation/trace IDs.
- **The three pillars: logs, metrics, traces**; what you'd add (Prometheus metrics, OpenTelemetry).
- Aggregations done in the DB vs the app; rollups/pre-aggregation for scale.

**Likely Qs:** "How would you debug a slow/failing task in prod?" · "How do these analytics scale to millions of rows?" (→ indexes, rollups).

---

## 12. Go language essentials

**Study:** interfaces (small, consumer-defined — your `Dispatcher`/`Publisher`), error handling (sentinel errors, `errors.Is`), `defer`/`recover`, struct embedding, slices/maps, `context`, the standard `net/http` + `database/sql`, GORM basics, modules.

**Likely Qs:** "How does Go error handling work?" · "What's a goroutine leak and how do you avoid it?" · "Why depend on an interface, not a concrete type?"

---

## 13. Deployment & ops (upcoming — Phase 9/10)

**Will build:** Docker multi-stage builds, docker-compose, **nginx** load balancer + reverse proxy.

**Study:** containers vs VMs, multi-stage Docker images, reverse proxy / load balancing, **nginx `limit_req`** (the edge rate-limit layer), env-based config, 12-factor app, health checks, zero-downtime deploys.

---

## The 60-second pitch (memorize)

> *"A from-scratch distributed background job engine in Go. Clients submit tasks over an API; they live
> in a MySQL table that IS the queue; a pool of worker goroutines across multiple instances claims and
> runs them concurrently with `SELECT ... FOR UPDATE SKIP LOCKED` (so no job runs twice); a DRR
> scheduler keeps tenants fair; failures retry then dead-letter; it survives crashes via startup
> recovery + a watchdog + graceful drain; Redis backs a cost-weighted rate limiter and a live worker
> registry; and a React dashboard streams it all over SSE."*

---

*Living document. Add topics as the project grows. Priority for revision: sections 1, 2, 3, 4, 7, 8 (⭐).*
