# 04 — Frontend (React)

The frontend is a **real-time dashboard**. It is deliberately kept clean and simple — the backend is the star. This doc covers structure, the SSE live-update mechanism, and the charts.

> **Why React (not Angular):** easier to learn, more in-demand in job postings, bigger community for getting unstuck. We build it so you understand every line and can defend it. The frontend is a *supporting* piece — clean and functional beats fancy.

---

## Stack

| Piece | Choice | Why |
|-------|--------|-----|
| Build tool | **Vite** | Fast, dead-simple, modern default (`npm create vite@latest`) |
| Framework | **React** (plain, no Next.js) | Keep concepts minimal for a rookie |
| Routing | **react-router-dom** | Switch between Dashboard / Tasks / Analytics |
| HTTP | **fetch** (or `axios`) | No heavy data lib needed to start |
| Live updates | **EventSource** (browser built-in) | Native SSE client — `new EventSource(url)` |
| Charts | **Recharts** | Declarative React charts; hand it data, it draws |
| Styling | Plain **CSS** / CSS modules | No pressure; can add a light UI kit later |

## Structure

```
frontend/
  src/
    main.jsx                 ← React entry
    App.jsx                  ← routes + layout
    api/
      client.js              ← fetch wrapper, injects x-api-key header
      tasks.js               ← task API calls
      analytics.js           ← analytics API calls
    hooks/
      useSSE.js              ← custom hook: subscribe to /api/sse/events
      useTasks.js            ← fetch + manage task list state
    pages/
      Dashboard.jsx          ← live task cards + worker utilization
      TaskManagement.jsx     ← submit form + filterable/paginated table + cancel/retry
      Analytics.jsx          ← 4 charts
    components/
      TaskCard.jsx
      TaskTable.jsx
      SubmitTaskForm.jsx
      ProgressBar.jsx        ← priority-colored progress
      WorkerPanel.jsx        ← which instance/worker is doing what
      charts/
        ExecutionTimeChart.jsx
        ThroughputChart.jsx
        FailureRateChart.jsx
        QueueWaitChart.jsx
    constants.js             ← API_KEYS list, task types, base URL
  index.html
  vite.config.js
  Dockerfile                 ← build static files, serve via nginx
```

## The three pages

1. **Dashboard** — the live view. Task cards that update in real time via SSE; a worker/instance panel showing which backend copy is processing what (the distributed proof). This is your **demo screen**.
2. **Task Management** — a reactive form to submit tasks; a filterable, paginated table; cancel/retry buttons; client-side text search (by id, type, client_id, status, error).
3. **Analytics** — 4 Recharts panels: execution time, throughput, failure rate, queue wait.

## How real-time works (the part that feels like magic but isn't)

`EventSource` is a browser API that opens a one-way stream to the server. The server pushes lines of text; the browser fires an event each time. In React we wrap it in a custom hook:

```jsx
// useSSE.js (shape)
import { useEffect } from "react";

export function useSSE(url, onEvent) {
  useEffect(() => {
    const es = new EventSource(url);
    es.onmessage = (e) => {
      try {
        onEvent(JSON.parse(e.data));   // wrap in try/catch so one bad
      } catch (_) { /* ignore malformed */ }  // message can't kill the stream
    };
    es.onerror = () => { /* browser auto-reconnects */ };
    return () => es.close();           // cleanup on unmount
  }, [url]);
}
```

Then a page just does: when an event arrives, update the matching task in state → React re-renders → the card/table updates live. **No polling.**

> ⚠️ Always wrap `JSON.parse(e.data)` in try/catch — a single malformed message must not break the whole stream.

## How charts update live

Recharts takes a data array as a prop and redraws when it changes. So: SSE delivers new data → you update the chart's state array → Recharts redraws. ~10–15 lines of glue per chart. You write **zero** drawing/math code.

```jsx
<LineChart data={throughputData}>
  <XAxis dataKey="time" /><YAxis />
  <Line dataKey="completed" />
</LineChart>
```

## API key handling

The 5 test keys live in `constants.js`. The dashboard lets you pick which client you're acting as; the `api/client.js` wrapper injects the chosen key as the `x-api-key` header on every request. (For SSE, the key can be passed as a `?apiKey=` query param since EventSource can't set headers.)

## Keep-it-simple rules

- Don't add Redux/global-state libraries yet — React's built-in `useState`/`useContext` is plenty.
- Don't add server-side rendering (Next.js) — it adds concepts you don't need.
- Don't over-style — a clean card layout + readable charts is enough for a great demo screenshot.

→ Next: [05 — Setup](05-SETUP.md)
