# 05 — Setup

Everything you need installed, every library, every command, and the environment variables. This is your "get the dev environment running" checklist.

---

## Tools to install on your machine (one-time)

| Tool | What for | Install (macOS) | Check |
|------|----------|-----------------|-------|
| **Go** (1.22+) | Backend language | `brew install go` | `go version` |
| **Node.js** (20+) | Frontend build | `brew install node` | `node -v` |
| **Docker Desktop** | Run MySQL/Redis + full stack | [docker.com](https://www.docker.com/products/docker-desktop/) | `docker --version` |
| **A DB GUI** | Inspect the database visually | TablePlus / DBeaver | — |
| **Git** | Version control | `brew install git` | `git --version` |

> For local dev you can run **MySQL and Redis via Docker** (no need to install them on your machine directly). See [06 — Hosting](06-HOSTING.md) for the compose file.

## Backend libraries (Go modules)

From `backend/`, after `go mod init <module-name>`:

```bash
go get github.com/gin-gonic/gin                 # HTTP router
go get github.com/go-sql-driver/mysql           # MySQL driver
go get github.com/redis/go-redis/v9             # Redis client
go get github.com/google/uuid                   # instance IDs
go get github.com/joho/godotenv                 # load .env in dev
go get github.com/stretchr/testify              # test assertions
# slog (logging) and database/sql are in the standard library
```

## Frontend libraries (npm)

```bash
npm create vite@latest frontend -- --template react
cd frontend
npm install
npm install react-router-dom        # routing
npm install recharts                # charts
# fetch + EventSource are built into the browser — no install
```

## Environment variables

**Never hardcode secrets.** They go in a `.env` file (git-ignored) locally, and are injected by the host in production.

### Backend `.env`

```bash
# Server
PORT=8080
INSTANCE_ID=backend-1            # unique per copy (the distributed proof)

# Database
DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASSWORD=change-me
DB_NAME=task_engine

# Redis
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=

# Worker pool
WORKER_COUNT=8
HUNG_TASK_TIMEOUT_SECONDS=60
WATCHDOG_INTERVAL_SECONDS=15

# Rate limiting
RATE_LIMIT_PER_MINUTE=10

# Task simulation
TASK_FAILURE_RATE=0.2           # 20% of tasks randomly fail
```

### Frontend `.env`

```bash
VITE_API_BASE_URL=http://localhost:8080   # backend URL (the nginx LB URL in prod)
```

> ⚠️ Add `.env` to `.gitignore`. Commit a `.env.example` (same keys, dummy values) so others know what's needed.

## Dev commands (cheat sheet)

```bash
# --- Backend --- (flat layout: main.go is at the backend root)
cd backend
go mod tidy                 # sync dependencies
go run .                    # run the server (migrates + seeds on startup)
go build .                  # compile
go test ./...               # run all tests
go test -cover ./...        # tests + coverage %

# --- Frontend ---
cd frontend
npm run dev                 # Vite dev server (http://localhost:5173)
npm run build               # production build (static files)
npm run preview             # preview the production build

# --- Infra (MySQL + Redis only, for local dev) ---
docker compose up mysql redis -d

# --- Full stack ---
docker compose up --build   # everything (http://localhost)
```

## First-run order (local)

> Local dev uses Homebrew-managed MySQL + Redis (`brew services start mysql redis`),
> not Docker. Docker is only used for the containerized/hosting path (Phase 9).

1. Ensure MySQL + Redis are running (`brew services list`), and create the DB once:
   `CREATE DATABASE task_engine;` (in Workbench or `mysql` CLI).
2. `cd backend && go run .` — start the backend. On startup it **auto-migrates**
   the tables (GORM `AutoMigrate`) and **auto-seeds** clients + sample tasks if the
   DB is empty. No separate migration/seed step.
3. `cd frontend && npm run dev` — start the dashboard.
4. Open the dashboard, watch tasks process live.

## `.gitignore` essentials

```
# secrets
.env
*.env
!.env.example

# go
/backend/server
*.test

# node
/frontend/node_modules
/frontend/dist

# os
.DS_Store
```

→ Next: [06 — Hosting](06-HOSTING.md)
