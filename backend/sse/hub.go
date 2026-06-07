// Package sse implements a Server-Sent Events hub: it holds open dashboard
// connections and pushes task-lifecycle events to them as they happen, so the UI
// updates live without polling.
package sse

import (
	"sync"
	"time"
)

// Event is one task-lifecycle notification pushed down an SSE connection.
type Event struct {
	Type     string `json:"type"`      // "task.started" | "task.completed" | "task.failed"
	TaskID   uint64 `json:"task_id"`
	ClientID string `json:"client_id"`
	Status   string `json:"status"`
	Time     int64  `json:"time"` // unix seconds
}

// Subscriber is one connected dashboard (one open SSE connection).
type Subscriber struct {
	clientID string
	ch       chan Event
}

// Events is the channel the SSE handler reads from to stream to the browser.
func (s *Subscriber) Events() <-chan Event { return s.ch }

// Hub fans task events out to connected dashboards, filtered per client (multi-tenant:
// a client only ever sees events for its own tasks). Safe for concurrent use.
type Hub struct {
	mu   sync.RWMutex
	subs map[*Subscriber]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[*Subscriber]struct{})}
}

// Subscribe registers a new connection for clientID and returns its handle. The
// channel is buffered so a brief burst of events doesn't block the publisher.
func (h *Hub) Subscribe(clientID string) *Subscriber {
	s := &Subscriber{clientID: clientID, ch: make(chan Event, 16)}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	return s
}

// Unsubscribe removes a connection (call on disconnect) and closes its channel.
func (h *Hub) Unsubscribe(s *Subscriber) {
	h.mu.Lock()
	delete(h.subs, s)
	h.mu.Unlock()
	close(s.ch)
}

// PublishTaskEvent builds an Event and fans it to the matching client's subscribers.
// It implements worker.Publisher, so the worker pool can notify the hub without
// importing this package.
//
// Sends are NON-BLOCKING: if a dashboard is too slow to keep up, we drop the event
// rather than block the worker goroutine that's publishing. A dropped UI update is
// fine; a stalled worker is not.
func (h *Hub) PublishTaskEvent(eventType string, taskID uint64, clientID, status string) {
	e := Event{
		Type:     eventType,
		TaskID:   taskID,
		ClientID: clientID,
		Status:   status,
		Time:     time.Now().Unix(),
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for s := range h.subs {
		if s.clientID != clientID {
			continue // multi-tenant: only deliver to the task's owner
		}
		select {
		case s.ch <- e:
		default: // subscriber's buffer is full — drop rather than block
		}
	}
}
