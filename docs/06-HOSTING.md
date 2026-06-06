# 06 — Hosting & Deployment

How we get this live for ~$6/month, how to see the hosted database, and how to run the **3-copy distributed setup** that proves the system is genuinely distributed.

---

## What we're hosting

The whole stack is **Docker containers**. We don't "host a Dockerfile" — we host the **Docker Compose stack** (multiple containers running together):

```
backend × 3   (Go app, identical copies)
frontend × 1  (React static files served by nginx)
nginx (LB)    (load balancer in front of the 3 backends)
mysql × 1     (shared queue + data)
redis × 1     (shared rate limiting)
```

## Cost — the honest numbers

| Option | Cost | Cold start? | Distributed cost | Verdict |
|--------|------|-------------|------------------|---------|
| **VPS** (DigitalOcean/Hetzner, 2GB) | **~$6/mo flat** | No | **Free** (flat fee, any # of containers) | ✅ **Recommended** |
| Railway | ~$5–15/mo | No | Each copy billed separately | Easiest, pricier for 3 copies |
| Render (free) | $0 | **Yes (sleeps)** | — | ❌ Avoid for demos |

**Recommendation: a $6/mo 2GB VPS.** Go containers are tiny (~30–50MB each), so 3 backends + MySQL (~400MB) + Redis (~50MB) + nginx fit comfortably in 2GB. Flat fee means the distributed setup costs nothing extra. Bonus: *"I deployed a containerized stack to a Linux VPS"* is a real ops talking point.

> **You don't need it live 24/7 forever.** Host during active job-hunting and record a demo video + GIF so recruiters see it work even when it's off. Likely total spend across a job search: under $20.

## The docker-compose stack (conceptual)

```yaml
services:
  mysql:
    image: mysql:8
    environment: { MYSQL_ROOT_PASSWORD: ..., MYSQL_DATABASE: task_engine }
    volumes: [ "mysqldata:/var/lib/mysql" ]   # persist data

  redis:
    image: redis:7

  backend1:
    build: ./backend
    environment: { INSTANCE_ID: backend-1, DB_HOST: mysql, REDIS_ADDR: redis:6379, ... }
    depends_on: [ mysql, redis ]
  backend2:
    build: ./backend
    environment: { INSTANCE_ID: backend-2, ... }
  backend3:
    build: ./backend
    environment: { INSTANCE_ID: backend-3, ... }

  nginx:
    image: nginx
    volumes: [ "./nginx.conf:/etc/nginx/nginx.conf" ]   # load-balances backend1/2/3
    ports: [ "80:80" ]
    depends_on: [ backend1, backend2, backend3 ]

  frontend:
    build: ./frontend     # built static files served by its own nginx, or merged into the LB

volumes: { mysqldata: {} }
```

The nginx config defines an `upstream` with the 3 backends and round-robins requests across them. (SSE needs `proxy_buffering off;` and the `/api/sse/` location block **before** the `/api/` block.)

## Deploying to a VPS (Style B — recommended for learning)

1. **Create a 2GB droplet/server** (Ubuntu) on DigitalOcean or Hetzner.
2. **SSH in**, install Docker + Docker Compose.
3. **Copy the project up** (`git clone` your repo, or `scp`).
4. Create the production `.env` files (real passwords — never committed).
5. `docker compose up -d --build` — the exact same command you run locally.
6. Point a **domain** (or just use the server IP) at it. Add HTTPS later with Caddy/Let's Encrypt if you want polish.

That's it — the same containers, on an always-on machine with a public address.

## Deploying to Railway (Style A — easiest)

1. Connect your GitHub repo.
2. Add services: backend (point at its Dockerfile), frontend, + a **MySQL plugin** and **Redis plugin**.
3. Set environment variables in the dashboard.
4. For the distributed demo, add 2 more backend services (backend-2, backend-3) and an nginx service — or note that Railway can scale replicas (billed per replica).

## Seeing the hosted database

A hosted DB is **not** a black box. The host gives you 5 values:

```
HOST  PORT  USER  PASSWORD  DATABASE
```

Use them to connect with a **GUI** (TablePlus / DBeaver) and browse tables visually — watch rows flip `pending → running → completed` live as the engine works. Or use the host's built-in data viewer. Great for demos *and* for grabbing real numbers (`SELECT COUNT(*) ...`).

**Security:** strong DB password (host-generated), never commit credentials to GitHub (env vars only).

## The distributed setup — making it visible & provable

The whole point of 3 copies: prove **distributed coordination**.

- All 3 share one **MySQL queue** → `SKIP LOCKED` guarantees no task runs twice.
- All 3 share one **Redis** → rate limits are global, not per-copy.
- **nginx** spreads incoming requests across the 3.

**The killer proof:** each copy has a unique `INSTANCE_ID`, and we record `processed_by` on every task. Then the dashboard/logs show:

```
Task #41 → backend-2
Task #42 → backend-1
Task #43 → backend-3
```

This turns "trust me" into "watch this." Screenshot/clip it for your demo video.

**Honest framing (don't overclaim):**
> *"I ran multiple backend instances behind a load balancer, all coordinating through a shared queue and Redis, demonstrating concurrency-safe distributed task processing."*

Not *"deployed across a global cluster"* — you don't need to, and it's easy to catch.

## How an interviewer "believes" the 3 copies

| Proof | Strength |
|-------|----------|
| You explain `SKIP LOCKED` + load balancing fluently | 🟢 Strongest — can't be faked |
| GitHub repo (compose file literally shows 3 backends + nginx) | 🟢 Checkable evidence |
| Live screen-share showing per-instance task processing | 🟢 Strong |
| Demo video + README architecture diagram | 🟡 Good backup |

The real proof is **understanding it so well that lying would be impossible.** That's why we build it so you own every piece.

→ Next: [07 — Roadmap](07-ROADMAP.md)
