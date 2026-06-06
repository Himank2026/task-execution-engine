# 09 — Resume Plan

How this project goes on your resume: which project to drop, the exact bullets, and how we fill in **real, measured** numbers (never invented).

---

## The swap

Replace **SmartPDF Processor** with this project. Keep **InspireBoard**.

| Project | Action | Reason |
|---------|--------|--------|
| SmartPDF Processor | ❌ Drop | Weakest; Streamlit reads as a prototype tool; vague metrics; doesn't match your Go-backend headline |
| InspireBoard | ✅ Keep | Your React/full-stack proof (JWT, CDN, live demo) — complements the backend project |
| **Task Execution Engine** | ✅ Add | Your backend centerpiece; matches your "Backend-Focused \| Go" headline |

Result: one heavy **backend-systems** project (Go) + one **full-stack web app** (React) = a clean, complementary pair that backs up your headline.

---

## The resume entry (final form)

> **Task Execution Engine — Distributed Background Job Processing System**
> *Go, Gin, MySQL, Redis, React.js, Docker, SSE* — Live Demo | GitHub
>
> - Built a **concurrent worker pool** in Go (**goroutines, channels**) processing **[X] tasks/sec** with automatic **crash recovery** and hung-worker detection, achieving **zero task loss** across forced worker failures.
> - Designed a **database-backed job queue** using MySQL `SELECT ... FOR UPDATE SKIP LOCKED` for concurrency-safe dequeue, **preventing duplicate processing** across parallel workers.
> - Implemented **fair scheduling (Deficit Round Robin)** to eliminate client starvation under **skewed load**, ensuring balanced task distribution across **[N] clients**.
> - Added **Redis-backed rate limiting** (sliding window, 10 req/min/client) and **real-time tracking via Server-Sent Events**, with a **React** dashboard rendering live execution analytics.

`[X]` and `[N]` get filled with measured numbers (see below).

---

## Differentiate from your NuvertOS bullet (don't undersell)

Your intern bullet and this project both touch "Go worker pools." Lean each into its superpower so they read as a **progression**, not a repeat:

- **Project = systems depth** (scheduling, crash recovery, distributed design, locking).
- **Intern job = production impact** (real users, real scale, business value — things a project can't claim).

Suggested tightened intern bullet (only keep numbers that are TRUE):
> *"Re-architected a production CSV ingestion pipeline in Go for 10+ voucher/invoice types in a live multi-tenant SaaS, using worker pools + batched DB writes to process 1000+ records/upload, cutting upload time by [X]% and replacing a workflow that failed at scale."*

Together they say: *applied it at work → went deeper on my own.*

---

## What we'll quantify (and how)

We measure with a load-test script (`bench/`) and the DB GUI. **Real numbers only** — a defensible "~800 tasks/sec on an 8-worker pool" beats a made-up "10,000 tasks/sec" every time.

### Print these (the catchiest ~5)
| Metric | Example phrasing | How measured |
|--------|------------------|--------------|
| Throughput | "~[X] tasks/sec" | Load test: submit N tasks, measure completion rate |
| Scaling factor | "[N]x throughput scaling 1→8 workers" | Run the bench at 1/2/4/8 workers |
| Zero loss + recovery | "0 task loss; recovery <[X]s across forced crashes" | Kill workers mid-run, measure |
| Fairness | "balanced across [N] clients under 100:1 skewed load" | Submit lopsided load, compare distribution |
| Rate limit + SSE | "10 req/min/client; live updates via SSE" | Burst test + dashboard |

### Keep as interview backup
- API latency (p50/p95/p99) under load
- Hung-worker detection time
- Retry success rate before dead-lettering
- Concurrent SSE connections supported
- Avg queue wait time
- # endpoints, # services, # task types

---

## Honesty guardrails (these protect you in interviews)

- ✅ "Sustained ~X tasks/sec across an 8-worker pool on a single 2GB VPS" — specific, scoped, defensible.
- ❌ "Processed millions of tasks with 99.99% uptime" — unverifiable, invites a grilling you'll lose.
- Every number must be one **you measured** and can **explain how**. If asked "how'd you get that?", you should have an answer.

---

## Portfolio polish (makes recruiters believe)
- **Live Demo link** (hosted on the VPS) — most candidates can't show a running distributed system.
- **Demo video / GIF** of the dashboard + tasks split across backend-1/2/3.
- **Clean README** with the architecture diagram (already in the main README).
- **Public GitHub** — the compose file literally shows the 3 backends + nginx, which is checkable proof.
