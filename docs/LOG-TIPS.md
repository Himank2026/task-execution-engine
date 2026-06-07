# Reading the logs — tips & tricks

> Our app logs with Go's `slog` in **structured** form: every line is a set of `key=value` (or JSON)
> fields, not a free-form sentence. That's great for machines but dense for humans. Here's how to make
> them readable.

---

## The quick win: `LOG_FORMAT=text` (readable mode)

We added a switch to the logger. By default it prints JSON (good for production tools). For local
dev, run with `LOG_FORMAT=text` and you get clean, short lines instead.

```bash
cd "/Users/himank/Desktop/Task Execution Engine - Distributed Background Job Processing System/backend"
go build -o task-engine . && LOG_FORMAT=text ./task-engine
```

**Before (default JSON) — hard to scan:**
```
{"time":"2026-06-07T12:03:52.159528+05:30","level":"INFO","msg":"task completed","worker":2,"task_id":143,"client":"alpha"}
```

**After (`LOG_FORMAT=text`) — easy to read:**
```
time=12:03:52 level=INFO msg="task completed" worker=2 task_id=143 client=alpha
```

Same data, short clock-time, no braces/quotes noise. This is the one to use day-to-day.

> Tip: you can combine it with other env vars, e.g.
> `LOG_FORMAT=text WATCHDOG_TIMEOUT_SECONDS=10 ./task-engine`.

---

## Filtering: only show the lines you care about with `grep`

`grep` prints only the lines that match a word. Pipe the app's output into it with `|`.

```bash
# only completed tasks
LOG_FORMAT=text ./task-engine | grep "task completed"

# anything about failures (failed + recovered panics)
LOG_FORMAT=text ./task-engine | grep -E "failed|panic"

# only warnings and errors (skip the routine INFO chatter)
LOG_FORMAT=text ./task-engine | grep -E "level=WARN|level=ERROR"

# everything about one specific task id
LOG_FORMAT=text ./task-engine | grep "task_id=143"
```

| Bit | Meaning |
|---|---|
| `\|` | **pipe** — feed the app's output into grep |
| `grep "x"` | print only lines containing `x` |
| `grep -E "a\|b"` | print lines containing `a` **or** `b` (the `-E` enables the `\|` "or") |
| `grep -v "x"` | **invert** — print lines that do NOT contain `x` (hide noise) |

> Note: when you pipe the app into `grep`, **Ctrl+C** still works to stop the app.

---

## Pretty-printing JSON mode with `jq`

If you keep the default JSON format, `jq` turns each blob into something readable. (`jq` is a separate
tool — install once with `brew install jq`.)

```bash
# pretty-print every log line as indented JSON
./task-engine | jq

# show ONLY the fields you care about, as a compact custom line
./task-engine | jq -r '"\(.time[11:19]) \(.level) \(.msg) task=\(.task_id // "-")"'
```
The second one prints lines like:
```
12:03:52 INFO task completed task=143
```
- `-r` = "raw" output (no surrounding quotes).
- `.time[11:19]` = take characters 11–19 of the timestamp (the `HH:MM:SS` part).
- `.task_id // "-"` = use `task_id`, or `-` if the line doesn't have one.

```bash
# only show ERROR-level lines, pretty-printed
./task-engine | jq 'select(.level=="ERROR")'

# only the messages, as plain text
./task-engine | jq -r '.msg'
```

> `LOG_FORMAT=text` is simpler for eyeballing; `jq` is more powerful when you want to *query* the logs
> (filter by field, reshape, count). Use whichever fits the moment.

---

## Saving logs to a file (to read later / search calmly)

```bash
# write logs to a file AND still see them on screen (tee = "T-split")
./task-engine 2>&1 | tee run.log

# then search the saved file at your leisure
grep "task_id=143" run.log
```
- `2>&1` = "send error-stream output to the same place as normal output" (so nothing is missed).
- `tee run.log` = duplicate the stream: one copy to the screen, one copy into `run.log`.

---

## A couple of reading habits that help

- **Watch the `msg` first.** Every line's `msg` is the headline (`task started`, `task completed`,
  `task failed`, `watchdog: recovered stuck task`, …). Skim those, then read fields only when a `msg`
  interests you.
- **`level` tells you severity:** `INFO` = normal, `WARN` = something notable but handled (a task
  failed and will retry), `ERROR` = something went wrong we want to know about.
- **Follow one `task_id`** through its life: `task started` → maybe `task failed` (retry) → eventually
  `task completed` or dead-lettered. Grepping a single `task_id=` is the fastest way to see one task's story.

---

*Living document — add new logging tricks here as we go.*
