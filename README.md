# Task Execution Engine — Distributed Background Job Processing System

> A distributed system for running long-running background jobs reliably — clients submit tasks, a pool of workers executes them across multiple backend instances, with fair scheduling, automatic failure recovery, and real-time tracking.

**One-line pitch (memorize this):**
*"It's a backend system for running long background jobs reliably — it queues them, runs them across a pool of workers, retries failures, and tracks everything live."*

---

## Why this project exists

This is a **resume centerpiece project** built to prove backend / distributed-systems ability for entry-level / new-grad software roles. The actual tasks are *simulated* (they sleep and randomly pass/fail) — the value is the **engine**: queuing, scheduling, worker management, fault tolerance, and real-time tracking. In a real deployment you'd swap the fake task handler for real work (sending emails, encoding video, generating reports) and **everything else stays the same.** The engine is reusable infrastructure — that's the point.

## Tech stack (every piece earns its place)

| Layer | Tech | Why it's here |
|-------|------|---------------|
| Backend | **Go** (goroutines, channels) | Concurrency is the heart of this project; Go is the idiomatic tool for worker pools |
| Web framework | **Gin** | Lightweight HTTP router for the API |
| Queue + data | **MySQL** | The task queue lives in a DB table; `SKIP LOCKED` makes dequeue concurrency-safe |
| Shared state | **Redis** | Global rate limiting shared across all backend copies |
| Frontend | **React** (Vite) | Real-time dashboard; chosen for learnability + job-market demand |
| Charts | **Recharts** | Live analytics graphs |
| Real-time | **SSE** (Server-Sent Events) | One-way server→browser push for live task updates |
| Deploy | **Docker + Docker Compose** | One sealed stack; runs the same locally and in production |
| Load balancer | **nginx** | Spreads requests across multiple backend copies |

**Deliberately NOT used:** Kafka (a DB-backed queue is sufficient and simpler), WebSockets (data only flows one way, SSE is simpler). Knowing what *not* to add is part of the design.

## Architecture at a glance

```
                    ┌──────────────┐
   Clients ───────▶ │ nginx (LB)   │
   (API keys)       └──────┬───────┘
              ┌────────────┼────────────┐
        ┌─────▼────┐ ┌─────▼────┐ ┌─────▼────┐
        │ backend1 │ │ backend2 │ │ backend3 │   ← identical Go copies
        │ workers  │ │ workers  │ │ workers  │     (worker pool each)
        └─────┬────┘ └─────┬────┘ └─────┬────┘
              └────────────┼────────────┘
                  ┌────────▼────────┐
                  │  MySQL (queue)  │  ← shared; SKIP LOCKED prevents double-pull
                  │  Redis (limits) │  ← shared; global rate limiting
                  └─────────────────┘
```

## Documentation index

Read these in order — they describe the whole system *before* we write code (the design-doc phase).

| # | Doc | What's in it |
|---|-----|--------------|
| 01 | [Architecture](docs/01-ARCHITECTURE.md) | System design, data flow, "why distributed", key decisions |
| 02 | [Database](docs/02-DATABASE.md) | The 3 tables, the queue mechanics, `SKIP LOCKED`, task lifecycle |
| 03 | [Backend](docs/03-BACKEND.md) | Go project structure, packages, worker pool, scheduler, services |
| 04 | [Frontend](docs/04-FRONTEND.md) | React structure, pages, components, SSE + charts |
| 05 | [Setup](docs/05-SETUP.md) | Tools to install, libraries, env vars, dev commands |
| 06 | [Hosting](docs/06-HOSTING.md) | Docker, VPS deploy, the 3-copy distributed setup, cost, DB access |
| 07 | [Roadmap](docs/07-ROADMAP.md) | Phase-by-phase build checklist |
| 08 | [Interview Defense](docs/08-INTERVIEW-DEFENSE.md) | Every design decision + the follow-up questions to expect |
| 09 | [Resume](docs/09-RESUME.md) | Resume bullets + how we'll quantify with real numbers |
| 10 | [Engineering Guide](docs/10-ENGINEERING-GUIDE.md) | How we write code: Git workflow, layered Go architecture, clean-code rules |

## Build philosophy

1. **Understand every piece** — the goal isn't just to ship it, it's to *defend* it in an interview. The build and the interview prep are the same activity.
2. **Phases, not big-bang** — foundation → core → workers → scheduler → real-time → tests → deploy → distributed.
3. **Real numbers only** — we measure throughput/latency/recovery with load tests; we never invent metrics.

## Status

📋 **Planning phase** — design docs first, then we build. See [Roadmap](docs/07-ROADMAP.md).
