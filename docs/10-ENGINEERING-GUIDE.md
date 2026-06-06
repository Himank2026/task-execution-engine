# 10 — Engineering Guide (How We Write Code)

This is the rulebook. It defines **how** we write, structure, branch, and ship code — the industry-standard way — so the codebase stays clean, consistent, and easy for anyone to read. When Claude writes code for this project, it follows this document.

> **The senior mindset:** code is read far more than it's written. Optimize for the next person (or future you) understanding it in 10 seconds. Boring, predictable, consistent code is *good* code.

---

## Part A — Git Workflow

### Branch model (Git Flow, simplified)

```
main   ──●─────────────●──────────────●─────▶   (stable / "safe" / production — always deployable)
          \           ↑              ↑
qa     ────●───●───●───●────●────●────●─────▶   (integration / testing — features land here first)
            \   \       /     \      /
feature/     ●───●─────/       ●────/           (one branch per feature)
```

| Branch | Role | Rule |
|--------|------|------|
| **`main`** | The stable, "safe", always-working branch. What you deploy. | Never commit directly. Only receives tested code promoted from `qa`. |
| **`qa`** | Integration + testing branch. New features land here to be tested together. | Features merge here first; test thoroughly before promoting to `main`. |
| **`feature/<name>`** | One branch per feature/fix. Where you actually write code. | Branch off `qa`. Short-lived. Delete after merge. |

> You wanted to call the stable branch "safe" — the *concept* is exactly right. We use the name **`main`** because it's the universal convention recruiters/teams recognize as "the good branch."

### The promotion ladder (every feature follows this)

```
1. git checkout qa && git pull              # start from latest qa
2. git checkout -b feature/retry-endpoint   # new branch per feature
3. ...write code, commit in small steps...
4. test LOCALLY until it works              # ← your rule, and it's the right one
5. open a Pull Request: feature → qa
6. self-review the PR (read your own diff!), merge
7. test on qa (features together)
8. when stable: Pull Request qa → main, merge
9. delete the feature branch
```

### Branch naming

```
feature/<short-description>     feature/worker-pool
fix/<short-description>         fix/sse-reconnect-leak
chore/<short-description>       chore/docker-compose-setup
docs/<short-description>        docs/architecture-update
```
Lowercase, hyphenated, descriptive. No `feature/test123` or `feature/my-branch`.

### Commit messages — Conventional Commits

Format: `type(scope): short imperative summary`

```
feat(worker): add hung-worker watchdog
fix(scheduler): prevent overlapping ticks with isScheduling guard
refactor(task): extract dequeue into repository layer
test(ratelimit): add sliding-window edge cases
docs(readme): add architecture diagram
chore(deps): bump go-redis to v9
```

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `perf`.

**Rules:**
- Imperative mood: "add", not "added"/"adds".
- One logical change per commit. Small, frequent commits beat giant ones.
- The summary fits in ~50 chars; add a body if the *why* needs explaining.

### Pull Request checklist (even solo)

- [ ] Title follows commit convention
- [ ] Description says **what** changed and **why**
- [ ] Code builds (`go build ./...`) and runs locally without errors
- [ ] No secrets/keys committed; `.env` is git-ignored
- [ ] Self-reviewed the diff line by line
- [ ] Tested locally (and noted how)

> **Why PRs when solo?** Reviewing your own diff catches bugs, documents intent, and produces a clean history recruiters can read. It's the single cheapest way to look professional on GitHub.

---

## Part B — Go Backend Architecture (Layered / Clean Architecture)

### The golden rule: each layer has ONE job, and dependencies point inward

A request flows **down** through layers, and each layer only knows about the one directly below it. Business logic never touches HTTP; HTTP never touches SQL.

```
   HTTP request
        │
        ▼
┌───────────────┐
│  main.go      │  wiring only: load config, open DB/Redis, build deps, start server
│ (backend root)│  NO business logic here
└───────┬───────┘
        ▼
┌───────────────┐
│  routes       │  maps URL + method → controller; attaches middleware
└───────┬───────┘
        ▼
┌───────────────┐
│  middleware   │  cross-cutting: auth (API key), rate limit, logging, recovery
└───────┬───────┘
        ▼
┌───────────────┐
│  controller   │  HTTP layer: parse/validate request → call service → format response
│ (controllers) │  NO business logic, NO SQL
└───────┬───────┘
        ▼
┌───────────────┐
│  service      │  business logic: rules, orchestration, decisions, + DB access
│  (services)   │  NO HTTP types
└───────┬───────┘
        ▼
┌───────────────┐
│  (repository) │  OPTIONAL: isolate SQL here if services grow (deferred to Phase 2)
└───────┬───────┘
        ▼
┌───────────────┐
│  model        │  plain structs (Task, etc.) — the shared data shapes
│  (models)     │
└───────┬───────┘
        ▼
     MySQL / Redis
```

> Our flow is exactly `main → routes → controllers → services → models`. SQL currently lives inside `services` to keep the layout simple. A dedicated **repository** layer (isolating all SQL between service and model) is an optional, professional refinement we may add in Phase 2 if the service files grow — it makes code more testable but adds a layer, so we add it only when it earns its place.

### Layer responsibilities (what goes where)

| Layer | Does | Never does |
|-------|------|------------|
| **main.go** | Load config, open DB/Redis, construct services/controllers (dependency injection), start HTTP server + worker pool, handle graceful shutdown | Business logic, SQL, request handling |
| **routes** | Register routes, group by prefix, attach middleware | Logic of any kind |
| **middleware** | Auth, rate limiting, logging, panic recovery, request IDs | Business decisions |
| **controllers** | Read/validate input (JSON, query, path params), call **one** service method, map result/error to HTTP status + JSON | SQL, business rules |
| **services** | The actual logic: validation rules, orchestration, retry decisions, parameterized SQL / DB access (incl. `SKIP LOCKED`), emitting SSE events | Touching `http.Request`/`gin.Context` |
| **repository** *(optional, Phase 2)* | If added: execute parameterized SQL, map rows → models, transactions — moved out of services | Business rules, HTTP concerns |
| **models** | Define structs and shared types | Logic |

### Concrete example — one feature top to bottom

`POST /api/tasks` (submit a task):

> Note: the example below shows the optional **repository** variant for teaching. In our current 3-layer setup the `repo.Insert` SQL lives directly inside `services/task.go` instead of a separate repository package.

```go
// 1. routes/task.go — wire the route
tasks := r.Group("/api/tasks", middleware.APIKey(authSvc), middleware.RateLimit(limiter))
tasks.POST("", taskController.Create)

// 2. controllers/task.go — HTTP only
func (h *TaskController) Create(c *gin.Context) {
    var req CreateTaskRequest
    if err := c.ShouldBindJSON(&req); err != nil {        // validate input
        c.JSON(http.StatusBadRequest, ErrorResponse{"invalid body"})
        return
    }
    clientID := c.GetString("client_id")                   // set by auth middleware
    task, err := h.svc.CreateTask(c.Request.Context(), clientID, req.Type, req.Priority, req.Payload)
    if err != nil {
        h.handleError(c, err)                              // map error → status
        return
    }
    c.JSON(http.StatusCreated, task)                       // format response
}

// 3. service/task.go — business logic
func (s *TaskService) CreateTask(ctx context.Context, clientID, taskType string, priority int, payload json.RawMessage) (*model.Task, error) {
    if priority < 1 || priority > 5 {                      // business rule
        return nil, ErrInvalidPriority
    }
    task := &model.Task{ClientID: clientID, Type: taskType, Priority: priority, Payload: payload, Status: model.StatusPending}
    if err := s.repo.Insert(ctx, task); err != nil {       // delegate persistence
        return nil, fmt.Errorf("create task: %w", err)     // wrap with context
    }
    s.sse.Broadcast(model.Event{Type: "task.created", Task: task})  // side effect
    return task, nil
}

// 4. repository/task.go — SQL only
func (r *TaskRepo) Insert(ctx context.Context, t *model.Task) error {
    const q = `INSERT INTO tasks (client_id, type, priority, payload, status, max_retries)
               VALUES (?, ?, ?, ?, ?, ?)`                  // parameterized, always
    res, err := r.db.ExecContext(ctx, q, t.ClientID, t.Type, t.Priority, t.Payload, t.Status, t.MaxRetries)
    if err != nil { return err }
    id, _ := res.LastInsertId()
    t.ID = id
    return nil
}

// 5. model/task.go — the shape
type Task struct {
    ID        int64           `json:"id" db:"id"`
    ClientID  string          `json:"client_id" db:"client_id"`
    Type      string          `json:"type" db:"type"`
    Priority  int             `json:"priority" db:"priority"`
    Payload   json.RawMessage `json:"payload" db:"payload"`
    Status    string          `json:"status" db:"status"`
    // ...
}
```

Notice: the handler never sees SQL, the repository never sees `gin.Context`, the service never imports `net/http`. **Clean separation.**

### Dependency Injection (how layers connect)

Build dependencies in `main.go` and pass them **down** (inject), don't create them inside layers. This makes everything testable (you can pass a fake repo to a service in a test).

```go
// main.go (backend root)
db := database.MustOpen(cfg)
taskSvc  := services.NewTaskService(db, sseHub)     // SQL lives in the service for now
taskCtrl := controllers.NewTaskController(taskSvc)
```

Services depend on **interfaces**, not concrete types, so they can be mocked:

```go
type TaskRepository interface {
    Insert(ctx context.Context, t *model.Task) error
    DequeueNext(ctx context.Context, instanceID string) (*model.Task, error)
    // ...
}
```

---

## Part C — Go Clean-Code Conventions

### Formatting & tooling (non-negotiable, automatic)
- **`gofmt`** / `go fmt ./...` — formatting is not a debate; the tool decides. Run on save.
- **`go vet ./...`** — catches suspicious code.
- **`golangci-lint`** — the standard linter; run before every PR.
- Tabs for indentation (gofmt does this), no manual alignment.

### Naming
- Packages: short, lowercase, no underscores (`task`, `worker`, `ratelimit`).
- Exported (public) identifiers: `PascalCase`. Unexported (private): `camelCase`.
- Interfaces often end in `-er` (`TaskRepository`, `Scheduler`).
- No stutter: in package `task`, name it `task.Service`, not `task.TaskService`.
- Be descriptive but not verbose: `dequeueNext`, not `dn` or `getTheNextTaskFromTheQueue`.

### Error handling (the Go way)
- Return errors, don't panic (panic only for truly unrecoverable startup failures).
- Handle errors immediately; don't ignore them (`_ =` only when truly intentional).
- **Wrap with context** as errors bubble up: `fmt.Errorf("dequeue task: %w", err)` — the `%w` preserves the original for `errors.Is`/`errors.As`.
- Define sentinel errors for known cases: `var ErrNotFound = errors.New("task not found")`.
- The handler is where errors become HTTP statuses — map them there, not deeper.

```go
if errors.Is(err, service.ErrNotFound) {
    c.JSON(http.StatusNotFound, ErrorResponse{"task not found"}); return
}
```

### Context
- Pass `context.Context` as the **first argument** to anything doing I/O (DB, Redis, HTTP).
- Propagate the request's context down so cancellations/timeouts flow through.

### Logging
- Structured logging with `log/slog` (key-value, JSON in prod).
- Log at boundaries (request in/out, task start/finish, errors). Don't spam inside loops.
- Never log secrets, full payloads, or API keys.

```go
slog.Info("task completed", "task_id", t.ID, "client", t.ClientID, "instance", instanceID)
```

### General hygiene
- **Small functions, single responsibility.** If it doesn't fit on a screen, split it.
- **No magic numbers/strings** — use named constants (`const WatchdogInterval = 15 * time.Second`).
- **Early returns** over deep nesting (guard clauses at the top).
- **One place for each thing** (e.g., cleanup logic lives in the supervisor only — never duplicated).
- Comments explain **why**, not **what** (the code already says what). Document every exported symbol.
- Keep functions pure where possible; isolate side effects (DB writes, SSE broadcasts).

---

## Part D — Manual Testing / QA

We test **manually and locally** — no automated unit tests. The `qa` branch is where each feature gets verified by hand before it's promoted to `main`. (This is a deliberate scope choice to keep the project focused on what we fully understand.)

How we verify a feature works before merging:
- **Run it locally** end-to-end: start the stack, exercise the new endpoint/feature, watch it behave.
- **Hit the API with `curl` or Postman** — confirm correct responses and HTTP status codes.
- **Watch the database** in your GUI (TablePlus/DBeaver) — confirm rows change as expected (`pending → running → completed`, retries, dead-letter).
- **Watch the dashboard** — confirm live SSE updates reflect what's happening.
- **Read the logs** — confirm no errors and that the right events are logged.
- For worker/distributed features, manually **kill a worker** or **submit skewed load** and observe recovery and fairness.

Keep a short checklist per feature of "what *should* happen", and confirm each point by hand before merging `feature → qa`, then again on `qa` before `qa → main`.

> Honest interview answer if asked about tests: *"I tested it manually end-to-end; adding automated tests would be my next step."* That's a fine answer for this scope.

---

## Part E — The "Add a Feature" Checklist (tie it all together)

Every new feature follows this exact flow:

1. `git checkout qa && git pull`
2. `git checkout -b feature/<name>`
3. Design which layers change: **model → repository → service → handler → router** (build bottom-up).
4. Write the code following Part B & C.
5. `gofmt`, `go vet`, `golangci-lint` — all clean.
6. Test **locally** end-to-end by hand (Part D — your rule, correct).
7. Commit in small Conventional Commits.
8. Open PR `feature → qa`, self-review, merge.
9. Verify on `qa`; when stable, PR `qa → main`.
10. Delete the feature branch.

> Build features **bottom-up** (model → repo → service → handler) so each layer you write can call into something that already exists and is tested.

---

This guide is the source of truth for code style and workflow. If something here conflicts with how code gets written later, this document wins — update it deliberately, not by drifting.
