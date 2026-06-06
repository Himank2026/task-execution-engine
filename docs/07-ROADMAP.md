# 07 — Build Roadmap

The phase-by-phase plan. We build in order, smallest working thing first, testing as we go. Each phase ends with something that actually runs. Check boxes as we complete them.

---

## Guiding principle

**Always have something that works.** We never go 3 days with a broken build. Each phase is a milestone you could demo (even if small). And we build it so you *understand* each piece — the build and the interview prep are the same activity.

---

## Phase 0 — Project skeleton
- [ ] Create repo, `go mod init`, folder structure (see [03 — Backend](03-BACKEND.md))
- [ ] `.gitignore`, `.env.example`, README
- [ ] `docker compose up mysql redis -d` works locally
- [ ] Backend "hello world" on `/api/health` returns 200

**Milestone:** server boots, health check responds.

## Phase 1 — Foundation
- [ ] `config` package (load env vars with defaults)
- [ ] `logger` (slog JSON)
- [ ] MySQL connection pool + ping
- [ ] Redis client + ping (with error listener)
- [ ] `types` package (Task struct, SSEEvent, etc.)
- [ ] Migration file `001_initial.sql` (3 tables) + a way to run it
- [ ] Seed script (~60 tasks across 5 clients)

**Milestone:** DB has tables + seed data; you can see them in TablePlus.

## Phase 2 — Core API (the queue)
- [ ] API key middleware (Redis-cached)
- [ ] `POST /api/tasks` (create → inserts `pending` row)
- [ ] `GET /api/tasks` (filters, pagination, whitelisted sorting)
- [ ] `GET /api/tasks/:id`
- [ ] `POST /api/tasks/:id/cancel`
- [ ] Centralized error handling

**Milestone:** you can submit + query tasks via `curl`/Postman.

## Phase 3 — Worker pool (the heart) ⭐
- [ ] Task service: `dequeue` with `SELECT ... FOR UPDATE SKIP LOCKED`
- [ ] Worker pool: N goroutines pulling from a channel
- [ ] Task handlers (simulated work: sleep + random pass/fail)
- [ ] `completeTask` / `failTask` (set `completed_at`!)
- [ ] Retry logic (`retry_count < max_retries`) + `dead_letter_queue`
- [ ] `processed_by` = INSTANCE_ID on every task

**Milestone:** submitted tasks actually get processed, retried, and dead-lettered.

## Phase 4 — Scheduler (fairness)
- [ ] Deficit Round Robin loop across clients
- [ ] `isScheduling` guard (no overlapping ticks)
- [ ] Orphan requeue on startup (`requeueTask`, no retry++)

**Milestone:** under skewed load, no client starves.

## Phase 5 — Fault tolerance
- [ ] Worker crash recovery (respawn + requeue in-flight task)
- [ ] Hung-worker watchdog (15s interval, 60s timeout)
- [ ] Graceful shutdown (SIGTERM → drain → close)

**Milestone:** kill a worker mid-task → task still completes, zero loss.

## Phase 6 — Real-time + analytics
- [ ] SSE hub (connection registry, broadcast, 15s heartbeat, snapshot on connect)
- [ ] `GET /api/sse/events`
- [ ] Analytics endpoints (execution-time, throughput, failure-rate, queue-wait)
- [ ] Rate limiter (sliding window, Redis + in-memory fallback)

**Milestone:** events stream live; analytics queries return data.

## Phase 7 — Frontend (React)
- [ ] Vite + React + router scaffold
- [ ] API client (inject `x-api-key`)
- [ ] `useSSE` hook
- [ ] Dashboard (live task cards + worker/instance panel)
- [ ] Task Management (submit form + table + cancel/retry + search)
- [ ] Analytics (4 Recharts panels)

**Milestone:** open the dashboard, watch tasks process live.

## Phase 8 — Manual QA pass
- [ ] Verify scheduler fairness by hand (submit skewed load, watch distribution)
- [ ] Verify crash recovery by hand (kill a worker mid-task, confirm zero loss)
- [ ] Verify rate limiting (burst requests, confirm 10/min cutoff)
- [ ] Verify no double-processing (watch `processed_by` under load)
- [ ] Walk every endpoint with `curl`/Postman + watch DB/logs

**Milestone:** every feature manually verified on the `qa` branch before promoting to `main`. (See [10 — Engineering Guide](10-ENGINEERING-GUIDE.md), Part D.)

## Phase 9 — Dockerize & deploy (single)
- [ ] Backend Dockerfile (multi-stage build)
- [ ] Frontend Dockerfile (build → nginx)
- [ ] `docker-compose.yml` (1 backend + frontend + mysql + redis)
- [ ] Deploy to VPS, get a public URL

**Milestone:** the app is LIVE on the internet.

## Phase 10 — Go distributed (the flex) ⭐
- [ ] Scale to 3 backend copies (unique INSTANCE_IDs)
- [ ] nginx load balancer (`upstream` with 3 backends; SSE buffering off)
- [ ] Dashboard shows `processed_by` per task (visible distribution)
- [ ] Verify no double-processing under load

**Milestone:** screenshot showing tasks split across backend-1/2/3.

## Phase 11 — Quantify & polish
- [ ] Write a load-test script (`bench/`) — submit N tasks, measure throughput
- [ ] Measure: throughput, scaling 1→8 workers, recovery time, fairness, latency
- [ ] Fill real numbers into [09 — Resume](09-RESUME.md)
- [ ] Record demo video + GIF, add architecture diagram to README

**Milestone:** real metrics + a polished, demo-able portfolio piece.

---

## Rough sequencing note

Phases 0–6 are the backend core (the bulk of the value). 7 is the UI. 8 hardens it. 9–10 make it live and distributed. 11 turns it into resume gold. We can move fast on the early phases since the design is already documented here.
