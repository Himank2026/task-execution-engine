package models

import (
	"encoding/json"
	"time"
)

// Task is the central row in our system: it's both a stored record AND a slot in
// the queue. A "pending" Task is a job waiting to run; a worker flips it through
// running -> completed/failed. GORM reads this struct's tags to build the
// `tasks` table, so the struct IS the schema.
type Task struct {
	// id BIGINT PK AUTO_INCREMENT. uint64 maps to BIGINT UNSIGNED; GORM treats a
	// field named ID as the primary key automatically.
	ID uint64 `gorm:"primaryKey" json:"id"`

	// Which client submitted this (derived from their API key). Indexed for
	// per-client queries and fair scheduling.
	ClientID string `gorm:"type:varchar(64);not null;index" json:"client_id"`

	// Task type — decides which handler runs it.
	Type string `gorm:"type:varchar(64);not null" json:"type"`

	// 1–5, where 5 = highest. Part of the composite dequeue index.
	Priority uint8 `gorm:"not null;default:1;index:idx_dequeue,priority:2" json:"priority"`

	// Arbitrary task input, stored in a real MySQL JSON column. json.RawMessage
	// is just []byte that stays as raw JSON (no pre-parsing into a Go map).
	Payload json.RawMessage `gorm:"type:json" json:"payload"`

	// Lifecycle state. ENUM keeps bad values out at the DB level. Leading column
	// of the composite dequeue index (we always filter status='pending' first).
	Status string `gorm:"type:enum('pending','running','completed','failed','cancelled');not null;default:pending;index:idx_dequeue,priority:1" json:"status"`

	RetryCount int `gorm:"not null;default:0" json:"retry_count"`
	MaxRetries int `gorm:"not null;default:3" json:"max_retries"`

	// Pointers (*string/*time.Time) so these can be NULL — they're empty until a
	// task fails or a worker touches it.
	ErrorMessage *string `gorm:"type:text" json:"error_message,omitempty"`
	ProcessedBy  *string `gorm:"type:varchar(64)" json:"processed_by,omitempty"`

	// created_at drives FIFO ordering (tie-break within a priority) and analytics.
	// Last column of the composite dequeue index. autoCreateTime: GORM sets it on
	// insert.
	CreatedAt   time.Time  `gorm:"autoCreateTime;index:idx_dequeue,priority:3" json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}
