package database

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"gorm.io/gorm"

	"github.com/Himank2026/task-execution-engine/backend/models"
)

// The 5 test clients from the design doc. The APIKey value is what a request
// sends in the x-api-key header.
var seedClients = []models.APIKey{
	{APIKey: "key-alpha", ClientID: "alpha", ClientName: "Alpha Corp", Active: true},
	{APIKey: "key-beta", ClientID: "beta", ClientName: "Beta Inc", Active: true},
	{APIKey: "key-gamma", ClientID: "gamma", ClientName: "Gamma LLC", Active: true},
	{APIKey: "key-delta", ClientID: "delta", ClientName: "Delta Co", Active: true},
	{APIKey: "key-test", ClientID: "test", ClientName: "Test Client", Active: true},
}

var seedTaskTypes = []string{"send_email", "resize_image", "generate_report", "call_webhook"}

// Seed inserts the test clients and ~60 sample tasks, but only if the database is
// empty. Running it on every startup is therefore safe: once seeded, it's a no-op.
func Seed(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.APIKey{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count api_keys: %w", err)
	}
	if count > 0 {
		return nil // already seeded
	}

	if err := db.Create(&seedClients).Error; err != nil {
		return fmt.Errorf("seed api_keys: %w", err)
	}

	// Weighted status mix: mostly completed, some pending, a few failed/running.
	statusPool := []string{
		"completed", "completed", "completed", "completed",
		"pending", "pending",
		"failed",
		"running",
	}

	tasks := make([]models.Task, 0, 60)
	for i := 0; i < 60; i++ {
		client := seedClients[rand.Intn(len(seedClients))]
		status := statusPool[rand.Intn(len(statusPool))]
		// Spread creation times over the last 12 hours for realistic analytics.
		created := time.Now().Add(-time.Duration(rand.Intn(720)) * time.Minute)
		payload, _ := json.Marshal(map[string]any{"seq": i, "demo": true})
		instance := "backend-1"

		t := models.Task{
			ClientID:   client.ClientID,
			Type:       seedTaskTypes[rand.Intn(len(seedTaskTypes))],
			Priority:   uint8(rand.Intn(5) + 1),
			Payload:    json.RawMessage(payload),
			Status:     status,
			MaxRetries: 3,
			CreatedAt:  created,
		}

		switch status {
		case "running":
			started := created.Add(time.Minute)
			t.StartedAt = &started
			t.ProcessedBy = &instance
		case "completed":
			started := created.Add(time.Minute)
			completed := started.Add(time.Duration(rand.Intn(120)+1) * time.Second)
			t.StartedAt = &started
			t.CompletedAt = &completed
			t.ProcessedBy = &instance
		case "failed":
			started := created.Add(time.Minute)
			// Doc rule: failed tasks MUST set completed_at, or throughput
			// analytics (completions per window) break.
			completed := started.Add(time.Duration(rand.Intn(120)+1) * time.Second)
			msg := "simulated failure during processing"
			t.StartedAt = &started
			t.CompletedAt = &completed
			t.RetryCount = rand.Intn(3)
			t.ErrorMessage = &msg
			t.ProcessedBy = &instance
		}

		tasks = append(tasks, t)
	}

	if err := db.Create(&tasks).Error; err != nil {
		return fmt.Errorf("seed tasks: %w", err)
	}

	return nil
}
