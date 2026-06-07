package services

import (
	"database/sql"
	"math"

	"gorm.io/gorm"

	"github.com/Himank2026/task-execution-engine/backend/models"
)

// AnalyticsService holds read-only reporting queries. It's separate from TaskService
// because its job is different: TaskService mutates the queue (create/claim/complete),
// AnalyticsService only aggregates and reads. Both just need the DB.
type AnalyticsService struct {
	db *gorm.DB
}

func NewAnalyticsService(db *gorm.DB) *AnalyticsService {
	return &AnalyticsService{db: db}
}

// Summary is the per-client metrics snapshot the dashboard renders.
type Summary struct {
	TotalTasks        int64            `json:"total_tasks"`
	StatusCounts      map[string]int64 `json:"status_counts"`       // {"completed": 12, ...}
	FailureRate       float64          `json:"failure_rate"`        // failed / (completed+failed), 0..1
	AvgExecutionMs    float64          `json:"avg_execution_ms"`    // avg run time of completed tasks
	AvgQueueWaitMs    float64          `json:"avg_queue_wait_ms"`   // avg wait before a worker started it
	CompletedLastHour int64            `json:"completed_last_hour"` // throughput proxy
}

// GetSummary computes the metrics for ONE client (scoped by client_id for multi-tenant
// isolation, exactly like the task queries).
func (s *AnalyticsService) GetSummary(clientID string) (*Summary, error) {
	summary := &Summary{StatusCounts: map[string]int64{}}

	// 1. Status breakdown (and total) — one GROUP BY pass.
	var statusRows []struct {
		Status string
		Count  int64
	}
	if err := s.db.Model(&models.Task{}).
		Select("status, COUNT(*) AS count").
		Where("client_id = ?", clientID).
		Group("status").
		Scan(&statusRows).Error; err != nil {
		return nil, err
	}
	for _, r := range statusRows {
		summary.StatusCounts[r.Status] = r.Count
		summary.TotalTasks += r.Count
	}

	// 2. Failure rate = failed / (completed + failed). (We exclude pending/running/
	//    cancelled — only finished work counts toward "did it succeed or fail".)
	completed := summary.StatusCounts["completed"]
	failed := summary.StatusCounts["failed"]
	if completed+failed > 0 {
		summary.FailureRate = round2(float64(failed) / float64(completed+failed))
	}

	// 3. Avg execution time: how long completed tasks actually ran.
	summary.AvgExecutionMs = s.avgMs(clientID,
		"status = 'completed' AND started_at IS NOT NULL AND completed_at IS NOT NULL",
		"started_at", "completed_at")

	// 4. Avg queue wait: how long tasks sat before a worker picked them up.
	summary.AvgQueueWaitMs = s.avgMs(clientID,
		"started_at IS NOT NULL",
		"created_at", "started_at")

	// 5. Throughput proxy: how many completed in the last hour.
	if err := s.db.Model(&models.Task{}).
		Where("client_id = ? AND status = 'completed' AND completed_at >= NOW() - INTERVAL 1 HOUR", clientID).
		Count(&summary.CompletedLastHour).Error; err != nil {
		return nil, err
	}

	return summary, nil
}

// avgMs returns the average of (endCol - startCol) in milliseconds for the client's
// tasks matching cond, or 0 if there are no matching rows.
//
// Note on safety: clientID is a parameterized `?` value (injection-safe). cond,
// startCol and endCol are concatenated into the SQL text, but they are ONLY ever the
// hardcoded literals passed by GetSummary above — never user input — so there's no
// injection surface here.
func (s *AnalyticsService) avgMs(clientID, cond, startCol, endCol string) float64 {
	var row struct{ Avg sql.NullFloat64 } // AVG over zero rows is NULL, hence NullFloat64

	q := "SELECT AVG(TIMESTAMPDIFF(MICROSECOND, " + startCol + ", " + endCol + ")) / 1000 AS avg " +
		"FROM tasks WHERE client_id = ? AND " + cond
	s.db.Raw(q, clientID).Scan(&row)

	if row.Avg.Valid {
		return round2(row.Avg.Float64)
	}
	return 0
}

// round2 rounds to 2 decimal places so the JSON is tidy (e.g. 1234.57 not 1234.5678).
func round2(f float64) float64 { return math.Round(f*100) / 100 }
