# Developer Toolkit — the terminal commands we use, explained

> **Why this doc exists.** While building this project we drive everything from the **terminal**:
> we hit the API with `curl`, inspect the database with `mysql`, and compile/run the app with `go`.
> If you've only ever clicked around a UI or Postman, this is the "what are all these commands?"
> reference. Each section says *what the tool is*, *why we use it*, and *every flag we actually type*.
>
> Mental model to start with: **the terminal (a.k.a. shell / command line) is just another way to
> tell your computer to run programs** — the same programs a GUI would run, but by typing their name
> instead of clicking an icon. `curl`, `mysql`, `go` are all just programs.

---

## 0. The single most important idea: an API is just HTTP

You thought you could only call the API from a UI or Postman. Here's the unlock:

> **A web API speaks a language called HTTP. ANYTHING that speaks HTTP can call it.**

Your React UI, Postman, `curl`, another backend service, a phone app — they're all just sending the
**same HTTP requests** to `http://localhost:8080/...`. The server can't even tell them apart. So:

- **Postman** = a GUI for sending HTTP requests (nice for exploring).
- **curl** = a *terminal* program for sending the exact same HTTP requests (nice for scripting/automation).
- **The React UI** = sends the same requests from JavaScript (`fetch`/`axios`).

They are three doors into the *same* house. We use `curl` because it's fast to type, scriptable
(loops!), and copy-pasteable into docs like this one.

An HTTP request has 4 parts, and you'll see all 4 in our curl commands:
1. **Method** — what you want to do: `GET` (read), `POST` (create), etc.
2. **URL** — what you're talking to: `http://localhost:8080/api/tasks`.
3. **Headers** — metadata: who you are (`x-api-key`), what format you're sending (`Content-Type`).
4. **Body** — the data (for POST): the JSON describing the task.

---

## 1. `curl` — calling the API from the terminal

`curl` ("client URL") sends an HTTP request and prints the response. This is how we test endpoints
without a UI.

### The anatomy of the commands we use

```bash
curl -X POST http://localhost:8080/api/tasks \
  -H "x-api-key: key-alpha" \
  -H "Content-Type: application/json" \
  -d '{"type":"send_email","priority":3}'
```

| Part | Meaning |
|---|---|
| `curl` | the program |
| `-X POST` | the **method**. `-X GET` to read, `-X POST` to create. (curl defaults to GET, so `-X` is optional for reads.) |
| `http://localhost:8080/api/tasks` | the **URL**. `localhost` = this same computer; `8080` = the port our server listens on. |
| `-H "x-api-key: key-alpha"` | a **header**. This one is our auth — it tells the server which client you are. |
| `-H "Content-Type: application/json"` | a **header** saying "my body is JSON." |
| `-d '{...}'` | the **data / body** — the JSON payload. Wrapped in single quotes so the shell leaves it alone. |

### The "quiet" flags we add when scripting

```bash
curl -s -o /dev/null -w "%{http_code}\n" -X POST http://localhost:8080/api/tasks -H "x-api-key: key-alpha" -H "Content-Type: application/json" -d '{"type":"send_email","priority":3}'
```

| Flag | Meaning |
|---|---|
| `-s` | **silent** — hide curl's progress bar and error chatter. |
| `-o /dev/null` | send the **response body to the trash** (`/dev/null` is the system's bottomless bin) — we don't care about the body, just the status. |
| `-w "%{http_code}\n"` | **write out** just the HTTP status code (and a newline `\n`). `201` = created, `200` = ok, `401` = bad/missing key, `404` = not found, `000` = couldn't even reach the server. |

### Our common curl recipes (copy-paste)

```bash
# Create one task
curl -s -X POST http://localhost:8080/api/tasks \
  -H "x-api-key: key-alpha" -H "Content-Type: application/json" \
  -d '{"type":"send_email","priority":3}'

# Create a task with limited retries (used for the panic test)
curl -s -X POST http://localhost:8080/api/tasks \
  -H "x-api-key: key-alpha" -H "Content-Type: application/json" \
  -d '{"type":"panic","priority":3,"max_retries":2}'

# List your tasks (GET; auth header still required)
curl -s http://localhost:8080/api/tasks -H "x-api-key: key-alpha"

# Get one task by id
curl -s http://localhost:8080/api/tasks/143 -H "x-api-key: key-alpha"

# Cancel a task
curl -s -X POST http://localhost:8080/api/tasks/143/cancel -H "x-api-key: key-alpha"

# Health check (no auth)
curl -s http://localhost:8080/api/health
```

> The seeded API keys are `key-alpha`, `key-beta`, `key-gamma`, `key-delta`, `key-test`
> (clients `alpha`, `beta`, …). Swap the key to act as a different client.

### `jq` (optional, nice-to-have)
Responses come back as one long line of JSON. Piping into `jq` pretty-prints it:
```bash
curl -s http://localhost:8080/api/tasks -H "x-api-key: key-alpha" | jq
```
(`|` is a **pipe** — it feeds one program's output into another. `jq` is a separate tool you may
need to install: `brew install jq`.)

---

## 2. `mysql` — inspecting the database from the terminal

Our queue *is* a MySQL table, so to see what's really happening we read the DB directly. You can do
this in **MySQL Workbench** (the GUI you already use) — the terminal `mysql` client runs the exact
same SQL, just typed instead of clicked.

### Two ways to run it

**One-shot (what I use in chat):** run a single query and exit, with `-e` ("execute"):
```bash
mysql -u root task_engine -e "SELECT status, COUNT(*) FROM tasks GROUP BY status;"
```
| Part | Meaning |
|---|---|
| `mysql` | the MySQL command-line client |
| `-u root` | connect as the **user** `root` |
| `task_engine` | the **database** to use (ours) |
| `-e "..."` | **execute** this SQL and print the result, then quit |

(No `-p` because locally root has no password. On a real server you'd add `-p` and it'd prompt.)

**Interactive:** just `mysql -u root task_engine`, then type SQL at the `mysql>` prompt, `exit;` to leave.

### The read-only queries I run to verify our work
> These only **read** — they never change data. Safe to run anytime.
```bash
# Count tasks by status (the overall health snapshot)
mysql -u root task_engine -e "SELECT status, COUNT(*) FROM tasks GROUP BY status;"

# Any tasks stuck 'running' with no live worker? (should be 0 after a clean shutdown)
mysql -u root task_engine -e "SELECT COUNT(*) AS stuck_running FROM tasks WHERE status='running';"

# Look at the most recent tasks
mysql -u root task_engine -e "SELECT id, type, status, retry_count, processed_by FROM tasks ORDER BY id DESC LIMIT 5;"

# What's in the dead-letter table (tasks that failed past max_retries)
mysql -u root task_engine -e "SELECT original_task_id, type, final_error FROM dead_letter_tasks ORDER BY id DESC LIMIT 5;"
```

### The one *write* query we used on purpose (the watchdog test)
This one **changes** data — we used it to fake a crashed instance's abandoned task:
```bash
mysql -u root task_engine -e "UPDATE tasks SET status='running', started_at=(NOW() - INTERVAL 5 MINUTE), completed_at=NULL, processed_by='dead-instance-99' WHERE id=(SELECT id FROM (SELECT id FROM tasks ORDER BY id DESC LIMIT 1) t);"
```
It flips the newest task to "running, started 5 minutes ago, owned by a dead instance" — exactly the
"stuck task" the watchdog hunts for. (The nested `SELECT ... FROM (SELECT ...)` is a MySQL quirk: you
can't directly `UPDATE` a table while sub-selecting from the same table, so we wrap it one layer.)

---

## 3. `go` — compiling and running the app

The Go toolchain. The commands we use, smallest to biggest:

| Command | What it does | When we use it |
|---|---|---|
| `go build ./...` | **Compile** everything; report errors. Produces no output if all good. (`./...` = "this folder and every subfolder".) | After every code change, to catch mistakes early. |
| `go vet ./...` | **Static analysis** — catches suspicious-but-compilable bugs (bad format strings, etc.). | Right after `go build`. |
| `go run .` | Compile **and** run in one step (temp binary). | Quick dev runs — but note the Ctrl+C quirk (section below). |
| `go build -o task-engine .` | Compile into a real file named `task-engine`. | When we want to run the actual binary. |
| `./task-engine` | **Run** that compiled binary. | Production-like runs (clean graceful shutdown). |
| `go mod tidy` | Sync the dependency list (`go.mod`) with what the code imports. | After adding/removing an import of an external library. |

### `go run .` vs `go build` + `./binary` (the thing you noticed)
- `go run .` = **two processes** (a wrapper + your app). On Ctrl+C the wrapper hands your shell prompt
  back *while your app is still shutting down*, so logs and the prompt interleave — looks broken, isn't.
- `go build -o task-engine . && ./task-engine` = **one process** (just your app). Ctrl+C goes straight
  to it; it drains and exits cleanly, prompt returns at the end. This is also how you'd run it in prod.
- `&&` means "run the next command only if the previous one succeeded" — so we only launch the binary
  if the build worked.

---

## 4. Handy shell building blocks you'll see

| Symbol / command | Meaning |
|---|---|
| `cd "path"` | **change directory** — move into a folder. Quotes are needed because our path has spaces. |
| `ls` | **list** files in the current folder. |
| `&&` | run the next command **only if** the previous one succeeded. |
| `\|` (pipe) | feed one program's **output** into the next program's **input** (e.g. `curl ... \| jq`). |
| `>` | **redirect** output to a file. `> /dev/null` throws it away. |
| `\n` | a newline character (used in curl's `-w`). |
| `Ctrl+C` | send the "interrupt" signal — politely asks the running program to stop (our app catches this and shuts down gracefully). |
| `Ctrl+\` | send the stronger "quit" signal — for when Ctrl+C is being ignored. |
| `for i in $(seq 1 12); do ...; done` | a **loop** — repeat a command 12 times (we use it to submit a burst of tasks). |

### The burst-submit loop, decoded
```bash
for i in $(seq 1 12); do
  curl -s -X POST http://localhost:8080/api/tasks \
    -H "x-api-key: key-alpha" -H "Content-Type: application/json" \
    -d '{"type":"send_email","priority":3}' > /dev/null
done
echo "submitted 12"
```
- `seq 1 12` produces the numbers 1..12; the loop body runs once per number.
- each iteration fires one POST (body thrown away with `> /dev/null`).
- ⚠️ `echo "submitted 12"` runs **no matter what** — even if every curl failed because the server was
  down. So "submitted 12" means "the loop finished", **not** "12 tasks were created". To truly confirm,
  check the status codes (`-w "%{http_code}"`) or count rows in the DB.

---

## 5. Quick "I just want to test the app" flow

```bash
# 1. go to the backend folder
cd "/Users/himank/Desktop/Task Execution Engine - Distributed Background Job Processing System/backend"

# 2. build + run (clean shutdown)
go build -o task-engine . && ./task-engine

# 3. in a SECOND terminal, submit a task
curl -s -X POST http://localhost:8080/api/tasks -H "x-api-key: key-alpha" -H "Content-Type: application/json" -d '{"type":"send_email","priority":3}'

# 4. watch it process in the first terminal's logs

# 5. check the DB
mysql -u root task_engine -e "SELECT id, type, status FROM tasks ORDER BY id DESC LIMIT 5;"

# 6. stop the server: press Ctrl+C in the first terminal (watch the graceful drain)
```

---

*Living document — add new tools/commands here as the project grows (Docker, redis-cli, etc.).*
