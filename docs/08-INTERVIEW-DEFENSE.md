# 08 — Interview Defense Sheet

Study this like flashcards. Every design choice, the plain-English reason, and the harder follow-up an interviewer will throw. If you can answer these, the project converts.

> **The #1 rule:** the proof that you built it is that you go *deeper* the more they ask. People who fake projects crumble on the second follow-up. You won't, because you'll actually understand it.

---

## Q: What is this project, in one sentence?
> "A backend system for running long background jobs reliably — it queues tasks, runs them across a pool of workers, retries failures, and tracks everything live."

## Q: Why Server-Sent Events (SSE) instead of WebSockets?
> "The data only flows one way — server to browser ('task 5 finished'). The browser never pushes back over that channel. SSE is simpler for one-way streaming; WebSockets' two-way channel would be unused complexity. SSE also auto-reconnects and works over plain HTTP."

**Follow-up — "What about polling?"**
> "Polling makes the browser repeatedly ask 'anything new?', which wastes requests. SSE lets the server push only when there's actually news."

**Follow-up — "How do you handle a dropped SSE connection?"**
> "EventSource auto-reconnects. On reconnect I send a snapshot of current state so the client re-syncs. I also wrap JSON.parse in try/catch so one malformed message can't kill the stream."

## Q: Why a MySQL table as the queue instead of Kafka / RabbitMQ?
> "At this scale a DB-backed queue is the right tradeoff — simpler, no extra infrastructure to run. I used `SELECT ... FOR UPDATE SKIP LOCKED` so multiple workers pull concurrently without ever processing the same task twice. It's a well-known pattern. If throughput grew into the millions of messages, I'd move to Kafka — but I'd want a real reason before taking on that complexity."

**Follow-up — "What exactly does SKIP LOCKED do?"**
> "`FOR UPDATE` locks the selected row inside the transaction. `SKIP LOCKED` tells other workers to skip rows that are already locked instead of waiting. So 8 workers hitting the queue at once each grab a different row — no collisions, no waiting."

**Follow-up — "What if a worker grabs a task then the process dies before finishing?"**
> "The task is left in 'running'. On startup I requeue orphaned 'running' tasks back to 'pending' — using a requeue that does NOT increment retry_count, because the task didn't fail, the process died. A watchdog also catches tasks stuck in 'running' past a timeout at runtime."

## Q: Why Deficit Round Robin for scheduling? Why not just priority?
> "Pure priority/FIFO lets one client hog all the workers — that's starvation. If Alpha submits 1,000 tasks, Beta waits forever. DRR takes fair turns across clients so everyone makes progress, while still honoring priority within each client's share."

**Follow-up — "How do you prevent two scheduler ticks overlapping?"**
> "An `isScheduling` guard flag — a new tick won't start until the current async tick finishes, so they don't stack up and double-dispatch."

## Q: Why Redis for rate limiting? Couldn't you use memory or the DB?
> "In-memory wouldn't work across multiple backend copies — each copy would have its own counter, so a client could bypass the limit by hitting different copies. Redis is a shared counter all copies use, so the limit is global. I used a sliding-window approach with a Redis sorted set. I didn't use MySQL because hitting the DB on every request adds load to the database that's already busy being the queue — Redis is in-memory and built for this."

**Follow-up — "What if Redis goes down?"**
> "I added an in-memory fallback — rate limiting degrades to per-instance instead of failing requests. Graceful degradation rather than a hard outage."

## Q: How is this 'distributed'? Isn't it just concurrent?
> "Good distinction. The worker pool alone is concurrent — one process doing many things. It's distributed because I run multiple identical backend copies behind an nginx load balancer, all coordinating through a shared MySQL queue (SKIP LOCKED stops double-processing) and a shared Redis (global rate limits). Each copy has an instance ID and records which one processed each task, so you can see the work spread across copies."

**Follow-up — "Is it truly distributed if they're on one machine?"** (be honest)
> "On one VPS it's not geographically distributed, but it demonstrates the distributed coordination — independent processes, shared queue, no double-processing, load balancing. The same design scales to multiple machines unchanged."

## Q: How does crash recovery work?
> "Each worker is supervised. If a worker goroutine panics or exits, the supervisor detects it, requeues the in-flight task so it isn't lost, and spawns a replacement to keep the pool full. Cleanup lives in one place so it never double-runs."

## Q: What's the retry logic exactly?
> "A failed task retries while `retry_count < max_retries`. With max_retries = 3 that's 3 retries plus the original attempt = 4 total. After that it moves to a separate dead_letter_queue table for inspection or manual retry."

## Q: How did you get your performance numbers?
> "I wrote a load-test script that submits N tasks and measures completion rate. I measured throughput, how it scaled from 1 to 8 workers, API latency percentiles, and crash-recovery time. [Quote your real measured numbers.]"

## Q: The tasks are fake — isn't that a weakness?
> "The tasks are simulated on purpose — the project's value is the engine, not the work. To make it real I'd replace the handler body with actual work like sending emails or encoding video, and the entire engine around it — queue, scheduler, workers, retries, tracking — stays identical. The engine is reusable infrastructure."

## Q: What would you improve / what are the limitations?
> "Honest answers: (1) tasks are simulated; (2) it's single-region; (3) the DB queue would eventually become a bottleneck at very high throughput, where I'd introduce Kafka; (4) I'd add metrics/observability (Prometheus) and proper auth beyond API keys for production." — *Showing you know the limits is a senior trait.*

---

## The 5 things that prove you built it
1. You can explain `SKIP LOCKED` and *why* it prevents double-processing.
2. You can explain DRR and the starvation problem it solves.
3. You can explain the Redis-vs-memory rate-limit reasoning + the fallback.
4. You can explain crash recovery and orphan requeue (and why requeue ≠ retry).
5. You can quote *measured* numbers and how you got them.

Master these five and you don't *claim* a distributed system — you *explain* one.
