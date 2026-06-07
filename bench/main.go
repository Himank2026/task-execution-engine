// Load test for the Task Execution Engine.
//
// It (1) clears all data, (2) injects N tasks via the demo-seed endpoint (which is NOT
// rate-limited, so we can flood the queue), then (3) polls until every task reaches a
// terminal state, and (4) reports throughput + latency. Run the same test against a
// 1-instance vs a 3-instance setup to see scaling.
//
// Usage:
//   cd bench && go run . [-url http://localhost:8080] [-batches 4]
//   (-batches × 50 = total tasks injected)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"time"
)

type summary struct {
	TotalTasks     int64            `json:"total_tasks"`
	StatusCounts   map[string]int64 `json:"status_counts"`
	AvgExecutionMs float64          `json:"avg_execution_ms"`
	AvgQueueWaitMs float64          `json:"avg_queue_wait_ms"`
}

func main() {
	url := flag.String("url", "http://localhost:8080", "base URL of the running stack")
	key := flag.String("key", "key-alpha", "any valid API key (just for auth)")
	batches := flag.Int("batches", 4, "number of 50-task batches to inject")
	flag.Parse()

	client := &http.Client{Timeout: 15 * time.Second}
	target := int64(*batches * 50)

	fmt.Printf("Target: %s  |  injecting %d tasks\n", *url, target)

	// 1. Clean slate.
	do(client, "POST", *url+"/api/demo/clear", *key)

	// 2. Inject, timing from the first insert.
	start := time.Now()
	for i := 0; i < *batches; i++ {
		do(client, "POST", *url+"/api/demo/seed", *key)
	}
	fmt.Printf("Injected in %.1fs — waiting for the queue to drain...\n", time.Since(start).Seconds())

	// 3. Poll until nothing is pending/running (terminal = completed/failed/cancelled).
	deadline := time.Now().Add(15 * time.Minute)
	var final summary
	for {
		s := getSummary(client, *url, *key)
		inflight := s.StatusCounts["pending"] + s.StatusCounts["running"]
		fmt.Printf("\r  %d/%d done, %d in flight   ",
			s.TotalTasks-inflight, s.TotalTasks, inflight)
		if s.TotalTasks >= target && inflight == 0 {
			final = s
			break
		}
		if time.Now().After(deadline) {
			fmt.Println("\n  timed out waiting for drain")
			final = s
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	wall := time.Since(start)
	fmt.Println()

	// 4. Report.
	fmt.Printf("\n========= RESULTS =========\n")
	fmt.Printf("Total tasks:     %d\n", final.TotalTasks)
	fmt.Printf("Completed:       %d\n", final.StatusCounts["completed"])
	fmt.Printf("Failed:          %d\n", final.StatusCounts["failed"])
	fmt.Printf("Wall-clock:      %.1fs\n", wall.Seconds())
	fmt.Printf("THROUGHPUT:      %.2f tasks/sec\n", float64(final.TotalTasks)/wall.Seconds())
	fmt.Printf("Avg execution:   %.2fs\n", final.AvgExecutionMs/1000)
	fmt.Printf("Avg queue wait:  %.2fs\n", final.AvgQueueWaitMs/1000)
	fmt.Printf("===========================\n")
}

func do(c *http.Client, method, url, key string) {
	req, _ := http.NewRequest(method, url, nil)
	req.Header.Set("x-api-key", key)
	if resp, err := c.Do(req); err == nil {
		resp.Body.Close()
	}
}

func getSummary(c *http.Client, base, key string) summary {
	var s summary
	req, _ := http.NewRequest("GET", base+"/api/analytics?all=true", nil)
	req.Header.Set("x-api-key", key)
	if resp, err := c.Do(req); err == nil {
		json.NewDecoder(resp.Body).Decode(&s)
		resp.Body.Close()
	}
	return s
}
