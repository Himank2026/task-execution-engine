# 07 — Build Roadmap

The phase-by-phase plan. We build in order, smallest working thing first, testing as we go. Each phase ends with something that actually runs. Check boxes as we complete them.

---

## Guiding principle

**Always have something that works.** We never go 3 days with a broken build. Each phase is a milestone you could demo (even if small). And we build it so you *understand* each piece — the build and the interview prep are the same activity.

---

## ✅ STATUS (2026-06-07)

**Phases 0–11 essentially DONE.** Built, dockerized (3 backends + nginx LB), load-tested with real
measured numbers, fully documented (README + diagram + BENCHMARKS + INTERVIEW-NOTES + STUDY-GUIDE).
**Only remaining:** deploy to a public URL (Phase 9 tail) + an optional demo video. Phase 8 manual-QA
is largely covered by the load test (zero-loss, no-double-processing, fairness all exercised).
**Measured:** ~4.6 tasks/sec (12 workers), ~2.5× scaling (1→3 instances), ~1.8s execution, 0 task loss.

Deliberate deviations from the original plan (not drift):
- **Flat backend layout** (`backend/main.go` + sibling packages); **GORM AutoMigrate** instead of a
  hand-written `001_initial.sql`; seed runs on startup when empty.
- **Phase 6** grew: analytics = summary + per-minute **throughput time-series** + **per-task-type
  breakdown**, each with an `?all=true` (all-clients) vs per-client **scope**. Rate limiter = Redis
  sliding-window (atomic **Lua** script). *In-memory fallback still TODO (currently fails open).*
- **Phase 7** = React + TypeScript + **Mantine** (dark, indigo) — 4 tabs (Dashboard / Tasks /
  Analytics / **Task Types**), live SSE feed, per-task **detail drawer**, client/type/status
  **filters**, **pagination**, **batch submit** (+ random mix), **Demo data** (append) / **Clear**
  buttons. (Free-text search + a retry button not built; filters cover the need.)
- **Phase 10 done early:** **Redis worker registry** (2s heartbeat, 6s TTL) so the worker panel
  aggregates **all instances**; run N copies via `INSTANCE_ID`/`PORT` sharing MySQL+Redis. *nginx LB
  + Docker still pending (Phase 9).*
- Per-phase "problems solved & how" lives in [`INTERVIEW-NOTES.md`](INTERVIEW-NOTES.md); terminal/Redis
  commands in [`DEV-TOOLKIT.md`](DEV-TOOLKIT.md); log tips in [`LOG-TIPS.md`](LOG-TIPS.md).

---

## Phase 0 — Project skeleton
- [x] Create repo, `go mod init`, folder structure (flat layout under `backend/`)
- [x] `.gitignore`, README
- [x] MySQL + Redis running locally (via Homebrew `brew services`, not docker compose)
- [x] Backend "hello world" on `/api/health` returns 200

**Milestone:** server boots, health check responds.

## Phase 1 — Foundation
- [x] `config` package (load env vars with defaults)
- [x] `logger` (slog JSON; + `LOG_FORMAT=text` dev mode)
- [x] MySQL connection pool + ping
- [x] Redis client + ping
- [x] `models` package (Task, APIKey, DeadLetterTask)
- [x] Schema via **GORM AutoMigrate** (not a hand-written `001_initial.sql`)
- [x] Seed script (~60 tasks across 5 clients, idempotent)

**Milestone:** DB has tables + seed data; you can see them in TablePlus.

## Phase 2 — Core API (the queue)
- [x] API key middleware (x-api-key → client_id; DB lookup, Redis cache deferred)
- [x] `POST /api/tasks` (create → inserts `pending` row) + `POST /api/tasks/batch` (bulk)
- [x] `GET /api/tasks` (filters, pagination, whitelisted sorting, `?all`/`?client` ops view)
- [x] `GET /api/tasks/:id`
- [x] `POST /api/tasks/:id/cancel`
- [x] Centralized error handling

**Milestone:** you can submit + query tasks via `curl`/Postman.

## Phase 3 — Worker pool (the heart) ⭐
- [x] Task service: `dequeue` with `SELECT ... FOR UPDATE SKIP LOCKED`
- [x] Worker pool: N goroutines pulling from a channel
- [x] Task handlers (simulated work: sleep 0.5–3s + 20% random fail; `panic` chaos type)
- [x] `completeTask` / `failTask` (set `completed_at`)
- [x] Retry logic (`retry_count < max_retries`) + dead-letter table
- [x] `processed_by` = INSTANCE_ID on every task

**Milestone:** submitted tasks actually get processed, retried, and dead-lettered.

## Phase 4 — Scheduler (fairness)
- [x] Deficit Round Robin loop across clients
- [x] `isScheduling` guard (no overlapping ticks)
- [x] Orphan requeue on startup (scoped to instance, no retry++)

**Milestone:** under skewed load, no client starves.

## Phase 5 — Fault tolerance
- [x] Panic recovery (`recover()` per task — one bad task can't crash the process)
- [x] Hung-worker watchdog (15s interval, 60s timeout; not instance-scoped → rescues dead peers)
- [x] Graceful shutdown (SIGTERM → drain via channel-close → close)

**Milestone:** kill a worker mid-task → task still completes, zero loss.

## Phase 6 — Real-time + analytics
- [x] SSE hub (per-client fan-out, non-blocking, 15s heartbeat) + `GET /api/sse/events`
- [x] Analytics: summary + throughput time-series + per-type breakdown (all `?all` scope-aware)
- [x] Rate limiter (Redis sliding window, atomic Lua) — *in-memory fallback still TODO (fails open)*

**Milestone:** events stream live; analytics queries return data.

## Phase 7 — Frontend (React)
- [x] Vite + React + TS + Mantine (dark/indigo) + router scaffold
- [x] API client (axios interceptor injects `x-api-key`)
- [x] `useSSE` hook (fetch-event-source, supports headers + auto-reconnect)
- [x] Dashboard (stat cards + live worker/instance panel + SSE feed)
- [x] Tasks (submit + batch/random-mix + table + cancel + filters + pagination + detail drawer)
- [x] Analytics (throughput area, latency line, status donut, priority bar) + Task Types tab

**Milestone:** open the dashboard, watch tasks process live.

## Phase 8 — Manual QA pass
- [ ] Verify scheduler fairness by hand (submit skewed load, watch distribution)
- [ ] Verify crash recovery by hand (kill a worker mid-task, confirm zero loss)
- [ ] Verify rate limiting (burst requests, confirm 10/min cutoff)
- [ ] Verify no double-processing (watch `processed_by` under load)
- [ ] Walk every endpoint with `curl`/Postman + watch DB/logs

**Milestone:** every feature manually verified on the `qa` branch before promoting to `main`. (See [10 — Engineering Guide](10-ENGINEERING-GUIDE.md), Part D.)

## Phase 9 — Dockerize & deploy (single)
- [x] Backend Dockerfile (multi-stage build → static binary on alpine)
- [x] Frontend Dockerfile (node build → nginx serve + reverse-proxy `/api`)
- [x] `docker-compose.yml` (backend + frontend + mysql + redis, healthchecks, one network) — `docker compose up --build` runs the whole stack at `localhost:8080`
- [ ] Deploy to a VPS, get a public URL — pending

**Milestone:** the app is LIVE on the internet.

## Phase 10 — Go distributed (the flex) ⭐
- [x] Scale to N backend copies (unique `INSTANCE_ID`/`PORT`, shared MySQL+Redis)
- [x] **Redis worker registry** (heartbeat + TTL) → dashboard aggregates ALL instances' workers
- [x] Dashboard shows `processed_by` / per-instance workers (visible distribution)
- [x] No double-processing under load (guaranteed by `SKIP LOCKED`)
- [x] nginx load balancer (`upstream backend_pool` round-robins backend1/2/3; SSE buffering off) — in `frontend/nginx.conf`; `docker compose up --build` runs 3 backends + LB

**Milestone:** screenshot showing tasks split across backend-1/2/3.

## Phase 11 — Quantify & polish
- [x] Write a load-test script (`bench/` — Go, stdlib only): clears, floods N tasks via demo-seed (bypasses rate limit), polls to drain, reports throughput + latency
- [x] Measure: **~4.6 tasks/sec** (12 workers) · **~2.5× scaling** (1→3 inst) · **~1.8s exec** · **0 task loss** (500-task run) — see [BENCHMARKS.md](BENCHMARKS.md)
- [x] Fill real numbers into [09 — Resume](09-RESUME.md) (no placeholders left)
- [x] Architecture diagram (Mermaid) + polished README; demo video/GIF still optional

**Milestone:** real metrics + a polished, demo-able portfolio piece.

---

## Rough sequencing note

Phases 0–6 are the backend core (the bulk of the value). 7 is the UI. 8 hardens it. 9–10 make it live and distributed. 11 turns it into resume gold. We can move fast on the early phases since the design is already documented here.
