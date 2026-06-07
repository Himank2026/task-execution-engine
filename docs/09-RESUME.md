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
> - Built a **concurrent worker pool** in Go (**goroutines, channels**) processing **~4.6 tasks/sec across 12 workers / 3 instances** (scaling **~2.5×** from one) with automatic **crash recovery** and hung-worker detection, achieving **zero task loss** across forced worker failures.
> - Designed a **database-backed job queue** using MySQL `SELECT ... FOR UPDATE SKIP LOCKED` for concurrency-safe dequeue, **preventing duplicate processing** across parallel workers.
> - Implemented **fair scheduling (Deficit Round Robin)** to eliminate client starvation under **skewed load**, ensuring balanced task distribution across **5 multi-tenant clients**.
> - Added **Redis-backed rate limiting** (sliding window, **cost-weighted by batch size**, atomic Lua script) and **real-time tracking via Server-Sent Events**, with a **React** dashboard rendering live execution analytics.
> - Containerized the full stack (**Docker Compose**) and ran **3 backend instances behind an nginx load balancer** sharing one MySQL/Redis — horizontally scalable with **zero coordination code** (the DB is the coordinator).

`[X]` and `[N]` get filled with measured numbers (see below / `BENCHMARKS.md`).

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

We measure with a load-test script (`bench/`) and the DB GUI. **Real numbers only** — a defensible
"~7 tasks/sec on 12 workers" beats a made-up "10,000 tasks/sec" every time. **Full methodology + every
quantifiable stat is in [BENCHMARKS.md](BENCHMARKS.md)** — run the bench, record your numbers there, then
mirror the catchiest into the resume bullets above.

### Print these (the catchiest ~5)
| Metric | Example phrasing | How measured |
|--------|------------------|--------------|
| Throughput | "~4.6 tasks/sec on 12 workers" | Load test: 200 tasks, completion rate (`bench/`) |
| Scaling factor | "~2.5x throughput scaling 1→3 instances" | bench at 1 (1.81/s) vs 3 (4.59/s) instances |
| Zero loss + recovery | "0 task loss; recovery <60s across forced crashes" | `kill -9` an instance mid-run |
| Fairness | "no starvation across [N] clients under skewed load" | Submit lopsided load, watch distribution |
| Rate limit + SSE | "cost-weighted limit (50 tasks/min/client); live updates via SSE" | Burst test + dashboard |

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
