# 02 — Database

The database does double duty: it stores the data **and** it *is* the task queue. This doc covers the schema (3 tables), the queue mechanics, and the all-important `SKIP LOCKED` pattern.

---

## Why MySQL is the queue (not a separate queue system)

A queue is just "a waiting line of jobs." You don't need special software for that — a table with a `status` column works perfectly. Submitting a task = insert a `pending` row. A worker taking a task = flip the oldest/highest-priority `pending` row to `running`. Finishing = flip to `completed`.

This is a real, named industry pattern: **"the database as a queue"** (a.k.a. job table / transactional outbox). It avoids running and maintaining heavy infrastructure (like Kafka) you don't need at this scale.

## The 3 tables

### 1. `tasks` — the main queue + source of truth

| Column | Type | Purpose |
|--------|------|---------|
| `id` | BIGINT PK AUTO_INCREMENT | Unique task id |
| `client_id` | VARCHAR | Which client submitted it (from API key) |
| `type` | VARCHAR | Task type (chooses which handler runs) |
| `priority` | TINYINT (1–5) | **5 = highest** |
| `payload` | JSON | Task input data |
| `status` | ENUM | `pending` / `running` / `completed` / `failed` / `cancelled` |
| `retry_count` | INT default 0 | How many times it has been retried |
| `max_retries` | INT default 3 | Retry ceiling |
| `error_message` | TEXT NULL | Last failure reason |
| `processed_by` | VARCHAR NULL | **Which backend instance ran it** (the distributed proof) |
| `created_at` | TIMESTAMP | When submitted (used for FIFO ordering + queue-wait analytics) |
| `started_at` | TIMESTAMP NULL | When a worker picked it up |
| `completed_at` | TIMESTAMP NULL | When it finished (success or terminal failure) |

**Indexes that matter:**
- `(status, priority, created_at)` — makes the dequeue query fast (the hot path).
- `(client_id)` — for per-client queries and fair scheduling.
- `(created_at)` — for analytics/time-range queries.

### 2. `dead_letter_queue` — tasks that failed too many times

Same shape as `tasks` (plus `failed_at`, `final_error`). When a task's `retry_count` reaches `max_retries`, it moves here. Kept separate so the main queue stays clean and you can inspect/retry failures deliberately.

### 3. `api_keys` — the clients

| Column | Purpose |
|--------|---------|
| `api_key` | The secret key in the `x-api-key` header |
| `client_id` | Human name (e.g. `alpha`) |
| `client_name` | Display name (e.g. "Alpha Corp") |
| `active` | Enable/disable a key |

Seeded with 5 test clients (Alpha, Beta, Gamma, Delta, Test).

## Task status lifecycle

```
        submit
          │
          ▼
      ┌────────┐   worker picks up    ┌─────────┐   handler ok   ┌───────────┐
      │ pending│ ───────────────────▶ │ running │ ─────────────▶ │ completed │
      └────────┘                      └─────────┘                └───────────┘
          ▲                                │
          │ retry (retry_count++)          │ handler error
          │                                ▼
          │                          retry_count < max_retries ?
          └──────────────── yes ───────────┤
                                            │ no
                                            ▼
                                   ┌──────────────────┐
                                   │ dead_letter_queue│
                                   └──────────────────┘

  (cancel) pending/running ──────────────▶ cancelled
```

## The critical part: concurrency-safe dequeue

**The problem:** multiple workers (and multiple backend copies) all ask "give me the next task" at the same instant. Without protection, two of them grab the same task → it runs twice.

**The solution — one line of SQL:**

```sql
SELECT id, client_id, type, priority, payload, retry_count, max_retries
FROM tasks
WHERE status = 'pending'
ORDER BY priority DESC, created_at ASC
LIMIT 1
FOR UPDATE SKIP LOCKED;
```

- `FOR UPDATE` — "I'm claiming this row; lock it so no one else touches it until my transaction ends."
- `SKIP LOCKED` — "if a row is already locked by another worker, don't wait — skip it and take the next available one."

**Result:** worker A locks task #42, worker B instantly skips #42 and grabs #43, worker C grabs #44. No collisions, no double-processing, no worker waiting. The database itself is the traffic cop.

Then in the same transaction we flip it:

```sql
UPDATE tasks SET status='running', started_at=NOW(), processed_by=? WHERE id=?;
COMMIT;
```

> ⚠️ This whole thing must be **one transaction**: `BEGIN → SELECT FOR UPDATE SKIP LOCKED → UPDATE → COMMIT`. The lock only holds inside the transaction.

## Important data rules (easy to get wrong)

- `failTask()` / `deadLetterTask()` **must set `completed_at`** — otherwise throughput analytics (which count completions per time window) break.
- `retryTask()` **must clear `started_at` and `completed_at`** — otherwise analytics show stale timing.
- **Retry math:** `retry_count < max_retries` (not `max_retries - 1`). "Max 3 retries" = 3 retries + 1 original attempt = **4 total attempts.**
- **Orphan requeue** (after a crash) uses `requeueTask()`, which does **NOT** increment `retry_count` — the task didn't fail, the process died. This is different from `retryTask()`.
- Seed data sets `completed_at` on failed/dead-lettered tasks to match real runtime behavior (so the charts look right immediately).

## Security note (SQL injection)

User-controllable sort columns/orders go through a **whitelist** (allowed column names only) and a forced `ASC`/`DESC` — never string-concatenated from user input. Everything else uses **parameterized queries** (`?` placeholders), never string formatting.

→ Next: [03 — Backend](03-BACKEND.md)
