package models

import "time"

// APIKey is a client credential. A request authenticates by sending its secret
// in the `x-api-key` header; we look it up here to learn which client (ClientID)
// is submitting work and whether the key is still allowed to.
type APIKey struct {
	ID uint64 `gorm:"primaryKey" json:"id"`

	// The secret sent in the x-api-key header. Unique so one key maps to one
	// client, and indexed because every authenticated request looks it up.
	APIKey string `gorm:"type:varchar(128);not null;uniqueIndex" json:"api_key"`

	// Machine-friendly id (e.g. "alpha") stamped onto every task this client
	// submits, and the human display name.
	ClientID   string `gorm:"type:varchar(64);not null;index" json:"client_id"`
	ClientName string `gorm:"type:varchar(128);not null" json:"client_name"`

	// A kill switch: flip to false to disable a key without deleting it.
	Active bool `gorm:"not null;default:true" json:"active"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}
