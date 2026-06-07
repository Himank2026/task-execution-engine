package services

import (
	"database/sql"
	"math"
	"strconv"
	"time"

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

// Summary is the metrics snapshot the dashboard renders. All counts are computed over
// the ENTIRE matching set (the whole table, or one client) — never a recent sample.
type Summary struct {
	TotalTasks        int64            `json:"total_tasks"`
	StatusCounts      map[string]int64 `json:"status_counts"`       // {"completed": 12, ...}
	PriorityCounts    map[string]int64 `json:"priority_counts"`     // {"1": 5, "2": 9, ...}
	FailureRate       float64          `json:"failure_rate"`        // failed / (completed+failed), 0..1
	AvgExecutionMs    float64          `json:"avg_execution_ms"`    // avg run time of completed tasks
	AvgQueueWaitMs    float64          `json:"avg_queue_wait_ms"`   // avg wait before a worker started it
	CompletedLastHour int64            `json:"completed_last_hour"` // throughput proxy
}

// GetSummary computes the metrics over the WHOLE matching set. If allClients is true
// it covers every client (the dashboard's global view); otherwise it's scoped to one
// client (multi-tenant isolation). Either way it's the full set, never a sample.
func (s *AnalyticsService) GetSummary(clientID string, allClients bool) (*Summary, error) {
	summary := &Summary{StatusCounts: map[string]int64{}, PriorityCounts: map[string]int64{}}

	// scope applies the per-client filter unless we're in all-clients mode.
	scope := func(q *gorm.DB) *gorm.DB {
		if allClients {
			return q
		}
		return q.Where("client_id = ?", clientID)
	}

	// 1. Status breakdown (and total) — one GROUP BY pass.
	var statusRows []struct {
		Status string
		Count  int64
	}
	if err := scope(s.db.Model(&models.Task{})).
		Select("status, COUNT(*) AS count").
		Group("status").
		Scan(&statusRows).Error; err != nil {
		return nil, err
	}
	for _, r := range statusRows {
		summary.StatusCounts[r.Status] = r.Count
		summary.TotalTasks += r.Count
	}

	// 2. Priority breakdown.
	var priorityRows []struct {
		Priority int
		Count    int64
	}
	if err := scope(s.db.Model(&models.Task{})).
		Select("priority, COUNT(*) AS count").
		Group("priority").
		Scan(&priorityRows).Error; err != nil {
		return nil, err
	}
	for _, r := range priorityRows {
		summary.PriorityCounts[strconv.Itoa(r.Priority)] = r.Count
	}

	// 3. Failure rate = failed / (completed + failed).
	completed := summary.StatusCounts["completed"]
	failed := summary.StatusCounts["failed"]
	if completed+failed > 0 {
		// Keep the raw ratio (no rounding) — the frontend formats it. Rounding a small
		// ratio like 2/605 to 2 decimals would collapse it to 0.00.
		summary.FailureRate = float64(failed) / float64(completed+failed)
	}

	// 4. Avg execution time + avg queue wait.
	summary.AvgExecutionMs = s.avgMs(clientID, allClients,
		"status = 'completed' AND started_at IS NOT NULL AND completed_at IS NOT NULL",
		"started_at", "completed_at")
	summary.AvgQueueWaitMs = s.avgMs(clientID, allClients,
		"started_at IS NOT NULL",
		"created_at", "started_at")

	// 5. Completed in the last hour.
	if err := scope(s.db.Model(&models.Task{})).
		Where("status = 'completed' AND completed_at >= NOW() - INTERVAL 1 HOUR").
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
func (s *AnalyticsService) avgMs(clientID string, allClients bool, cond, startCol, endCol string) float64 {
	where := cond
	args := []any{}
	if !allClients {
		where = "client_id = ? AND " + cond
		args = append(args, clientID)
	}

	var row struct{ Avg sql.NullFloat64 } // AVG over zero rows is NULL, hence NullFloat64
	q := "SELECT AVG(TIMESTAMPDIFF(MICROSECOND, " + startCol + ", " + endCol + ")) / 1000 AS avg FROM tasks WHERE " + where
	s.db.Raw(q, args...).Scan(&row)

	if row.Avg.Valid {
		return round2(row.Avg.Float64)
	}
	return 0
}

// round2 rounds to 2 decimal places so the JSON is tidy (e.g. 1234.57 not 1234.5678).
func round2(f float64) float64 { return math.Round(f*100) / 100 }

// Throughput time-series settings: 30 buckets of 10 seconds = the last 5 minutes.
const (
	bucketSeconds = 10
	bucketCount   = 30
)

// ThroughputPoint is one time-bucket on the throughput chart.
type ThroughputPoint struct {
	Time           string  `json:"time"`             // "HH:mm:ss" label for the x-axis
	Completed      int64   `json:"completed"`        // tasks completed in this bucket
	AvgExecutionMs float64 `json:"avg_execution_ms"` // avg run time of those tasks
	AvgQueueWaitMs float64 `json:"avg_queue_wait_ms"`
}

// GetThroughput returns completions-over-time for the last 5 minutes, bucketed into
// 10-second slots. We bucket by 10s (not by minute) so even a short burst of tasks
// produces a meaningful curve.
//
// The buckets are keyed by UNIX epoch seconds, which are timezone-independent — that
// sidesteps any Go/MySQL timezone mismatch when we zero-fill the empty buckets below.
func (s *AnalyticsService) GetThroughput(clientID string, allClients bool) ([]ThroughputPoint, error) {
	type bucketRow struct {
		BucketUnix int64
		Completed  int64
		Exec       sql.NullFloat64
		Wait       sql.NullFloat64
	}

	where := "status = 'completed' AND completed_at >= NOW() - INTERVAL 300 SECOND"
	args := []any{}
	if !allClients {
		where = "client_id = ? AND " + where
		args = append(args, clientID)
	}

	var rows []bucketRow
	err := s.db.Raw(`
		SELECT CAST(FLOOR(UNIX_TIMESTAMP(completed_at) / 10) * 10 AS SIGNED) AS bucket_unix,
		       COUNT(*) AS completed,
		       AVG(TIMESTAMPDIFF(MICROSECOND, started_at, completed_at)) / 1000 AS exec,
		       AVG(TIMESTAMPDIFF(MICROSECOND, created_at, started_at)) / 1000 AS wait
		FROM tasks
		WHERE `+where+`
		GROUP BY bucket_unix
	`, args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	byBucket := make(map[int64]bucketRow, len(rows))
	for _, r := range rows {
		byBucket[r.BucketUnix] = r
	}

	// Build all 30 buckets ending at "now", filling gaps with zeros so the chart shows
	// a continuous timeline instead of just the moments something happened.
	nowBucket := (time.Now().Unix() / bucketSeconds) * bucketSeconds
	points := make([]ThroughputPoint, 0, bucketCount)
	for i := bucketCount - 1; i >= 0; i-- {
		b := nowBucket - int64(i)*bucketSeconds
		p := ThroughputPoint{Time: time.Unix(b, 0).Format("15:04:05")}
		if r, ok := byBucket[b]; ok {
			p.Completed = r.Completed
			if r.Exec.Valid {
				p.AvgExecutionMs = round2(r.Exec.Float64)
			}
			if r.Wait.Valid {
				p.AvgQueueWaitMs = round2(r.Wait.Float64)
			}
		}
		points = append(points, p)
	}
	return points, nil
}

// TypeStat is the breakdown for one task type (e.g. "send_email").
type TypeStat struct {
	Type           string  `json:"type"`
	Total          int64   `json:"total"`
	Completed      int64   `json:"completed"`
	Failed         int64   `json:"failed"`
	Pending        int64   `json:"pending"`
	Running        int64   `json:"running"`
	Cancelled      int64   `json:"cancelled"`
	FailureRate    float64 `json:"failure_rate"`     // failed / (completed+failed)
	AvgExecutionMs float64 `json:"avg_execution_ms"` // avg run time of this type's completed tasks
	AvgQueueWaitMs float64 `json:"avg_queue_wait_ms"`
}

// GetTypeBreakdown aggregates per task type: counts by status, failure rate, and the
// average execution + queue-wait time. Computed over the whole matching set (all rows).
func (s *AnalyticsService) GetTypeBreakdown(clientID string, allClients bool) ([]TypeStat, error) {
	where := ""
	args := []any{}
	if !allClients {
		where = "WHERE client_id = ?"
		args = append(args, clientID)
	}

	type row struct {
		Type      string
		Total     int64
		Completed int64
		Failed    int64
		Pending   int64
		Running   int64
		Cancelled int64
		AvgExec   sql.NullFloat64
		AvgWait   sql.NullFloat64
	}
	var rows []row
	q := `
		SELECT type,
		       COUNT(*) AS total,
		       COUNT(CASE WHEN status = 'completed' THEN 1 END) AS completed,
		       COUNT(CASE WHEN status = 'failed' THEN 1 END) AS failed,
		       COUNT(CASE WHEN status = 'pending' THEN 1 END) AS pending,
		       COUNT(CASE WHEN status = 'running' THEN 1 END) AS running,
		       COUNT(CASE WHEN status = 'cancelled' THEN 1 END) AS cancelled,
		       AVG(CASE WHEN status = 'completed' AND started_at IS NOT NULL AND completed_at IS NOT NULL
		                THEN TIMESTAMPDIFF(MICROSECOND, started_at, completed_at) / 1000 END) AS avg_exec,
		       AVG(CASE WHEN started_at IS NOT NULL
		                THEN TIMESTAMPDIFF(MICROSECOND, created_at, started_at) / 1000 END) AS avg_wait
		FROM tasks ` + where + `
		GROUP BY type
		ORDER BY total DESC
	`
	if err := s.db.Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	stats := make([]TypeStat, 0, len(rows))
	for _, r := range rows {
		st := TypeStat{
			Type: r.Type, Total: r.Total,
			Completed: r.Completed, Failed: r.Failed, Pending: r.Pending,
			Running: r.Running, Cancelled: r.Cancelled,
		}
		if r.Completed+r.Failed > 0 {
			st.FailureRate = float64(r.Failed) / float64(r.Completed+r.Failed) // raw ratio; UI formats
		}
		if r.AvgExec.Valid {
			st.AvgExecutionMs = round2(r.AvgExec.Float64)
		}
		if r.AvgWait.Valid {
			st.AvgQueueWaitMs = round2(r.AvgWait.Float64)
		}
		stats = append(stats, st)
	}
	return stats, nil
}
