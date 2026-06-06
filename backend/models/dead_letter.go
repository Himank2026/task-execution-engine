package models

import (
	"encoding/json"
	"time"
)

// DeadLetterTask is where a Task lands after it fails more times than max_retries
// allows. We copy it out of the main `tasks` queue so that queue stays lean and
// only holds live work — failures are parked here for inspection or manual retry.
// Same core shape as Task, plus two columns about the final failure.
type DeadLetterTask struct {
	ID uint64 `gorm:"primaryKey" json:"id"`

	// The id this task had in the `tasks` table before it was moved here, so we
	// can trace it back to its original history.
	OriginalTaskID uint64 `gorm:"not null;index" json:"original_task_id"`

	ClientID string          `gorm:"type:varchar(64);not null;index" json:"client_id"`
	Type     string          `gorm:"type:varchar(64);not null" json:"type"`
	Priority uint8           `gorm:"not null;default:1" json:"priority"`
	Payload  json.RawMessage `gorm:"type:json" json:"payload"`

	RetryCount int `gorm:"not null;default:0" json:"retry_count"`
	MaxRetries int `gorm:"not null;default:3" json:"max_retries"`

	// The error that finally killed it, and which instance was running it.
	FinalError  *string `gorm:"type:text" json:"final_error,omitempty"`
	ProcessedBy *string `gorm:"type:varchar(64)" json:"processed_by,omitempty"`

	// When the task was originally created vs. when it was dead-lettered.
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	FailedAt  time.Time `gorm:"autoCreateTime;index" json:"failed_at"`
}
