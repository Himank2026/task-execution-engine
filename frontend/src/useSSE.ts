import { useEffect, useState } from 'react'
import { fetchEventSource } from '@microsoft/fetch-event-source'

import { getApiKey } from './api'

// The shape of a task event the backend pushes (matches sse.Event in Go).
export interface TaskEvent {
  type: string // "task.started" | "task.completed" | "task.failed"
  task_id: number
  client_id: string
  status: string
  time: number // unix seconds
}

// useSSE opens a live connection to the backend's event stream and returns the most
// recent events plus whether we're currently connected. We use fetch-event-source
// (not the browser's built-in EventSource) for ONE reason: EventSource can't send
// custom headers, but our endpoint needs the x-api-key header.
export function useSSE(maxEvents = 30) {
  const [events, setEvents] = useState<TaskEvent[]>([])
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    const ctrl = new AbortController() // lets us close the connection on unmount

    fetchEventSource('/api/sse/events', {
      headers: { 'x-api-key': getApiKey() },
      signal: ctrl.signal,
      // Keep the stream open even when the browser tab is in the background — without
      // this, switching to another tab/app drops and reconnects the connection.
      openWhenHidden: true,

      onopen: async (res) => {
        if (res.ok && res.headers.get('content-type')?.includes('text/event-stream')) {
          setConnected(true)
          return
        }
        throw new Error(`SSE failed to open: ${res.status}`) // bad response, stop
      },

      onmessage: (msg) => {
        if (msg.event === 'heartbeat') return // ignore keep-alive pings
        try {
          const data = JSON.parse(msg.data) as TaskEvent
          // newest first, capped so the list can't grow forever
          setEvents((prev) => [data, ...prev].slice(0, maxEvents))
        } catch {
          // ignore unparseable lines
        }
      },

      onerror: () => {
        setConnected(false)
        // returning nothing lets fetch-event-source retry automatically
      },
    })

    return () => ctrl.abort() // close the stream when the component goes away
  }, [maxEvents])

  return { events, connected }
}
