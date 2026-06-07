package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Himank2026/task-execution-engine/backend/config"
	"github.com/Himank2026/task-execution-engine/backend/database"
	"github.com/Himank2026/task-execution-engine/backend/logger"
	"github.com/Himank2026/task-execution-engine/backend/routes"
	"github.com/Himank2026/task-execution-engine/backend/ratelimit"
	"github.com/Himank2026/task-execution-engine/backend/scheduler"
	"github.com/Himank2026/task-execution-engine/backend/services"
	"github.com/Himank2026/task-execution-engine/backend/sse"
	"github.com/Himank2026/task-execution-engine/backend/watchdog"
	"github.com/Himank2026/task-execution-engine/backend/worker"
)

func main() {
	logger.Init()
	cfg := config.Load()

	db, err := database.ConnectMySQL(cfg)
	if err != nil {
		slog.Error("connect mysql", "err", err)
		os.Exit(1)
	}
	// *gorm.DB has no Close(); reach the underlying *sql.DB to close the pool.
	if sqlDB, err := db.DB(); err == nil {
		defer func() {
			slog.Info("closing mysql connection pool")
			sqlDB.Close()
			slog.Info("mysql connection pool closed")
		}()
	}
	slog.Info("connected to MySQL", "host", cfg.DBHost, "port", cfg.DBPort, "db", cfg.DBName)

	if err := database.Migrate(db); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}
	slog.Info("database migrated")

	if err := database.Seed(db); err != nil {
		slog.Error("seed", "err", err)
		os.Exit(1)
	}
	slog.Info("database seeded")

	rdb, err := database.ConnectRedis(cfg)
	if err != nil {
		slog.Error("connect redis", "err", err)
		os.Exit(1)
	}
	defer func() {
		slog.Info("closing redis connection")
		rdb.Close()
		slog.Info("redis connection closed")
	}()
	slog.Info("connected to Redis", "addr", cfg.RedisAddr)

	// One shared TaskService instance for the HTTP layer, the worker pool, AND the
	// scheduler — they all operate on the exact same business logic.
	taskService := services.NewTaskService(db)

	// Startup recovery: requeue any tasks this instance left stuck in "running" from
	// a previous crash, BEFORE the scheduler starts handing out work.
	if n, err := taskService.RequeueOrphanedTasks(cfg.InstanceID); err != nil {
		slog.Error("requeue orphaned tasks", "err", err)
	} else if n > 0 {
		slog.Info("requeued orphaned tasks", "count", n)
	}

	// SSE hub: the worker pool publishes task events to it; the HTTP layer streams
	// them to connected dashboards.
	hub := sse.NewHub()

	// Start the worker pool (execution): N goroutines running whatever they're given.
	pool := worker.NewPool(taskService, cfg.InstanceID, cfg.WorkerCount, hub)
	pool.Start()
	defer pool.Stop() // runs second (LIFO): drain workers after scheduler stops

	// Start the scheduler (fairness): DRR pass that feeds the pool fairly per client.
	sched := scheduler.NewScheduler(taskService, pool, cfg.InstanceID, cfg.SchedulerQuantum)
	sched.Start()
	defer sched.Stop() // runs second-to-first (LIFO): stop submitting before the pool closes

	// Start the watchdog (liveness): periodically requeues tasks stuck in "running"
	// past the timeout (a hung worker, or a peer instance that died).
	wd := watchdog.NewWatchdog(taskService, cfg.WatchdogInterval, cfg.WatchdogTimeout)
	wd.Start()
	defer wd.Stop() // runs first (LIFO): stop reclaiming before we drain on shutdown

	// Per-client API rate limiter (sliding window, backed by Redis).
	limiter := ratelimit.NewLimiter(rdb, cfg.RateLimitMax, cfg.RateLimitWindow)

	r := routes.SetupRouter(db, taskService, limiter, hub, pool)

	// Wrap the Gin router in a standard-library http.Server. r.Run() blocks forever and
	// gives us no way to stop it; an http.Server hands us Shutdown() for a clean,
	// drain-in-flight stop.
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Run the server in its own goroutine so main can move past it and sit waiting for a
	// shutdown signal. ListenAndServe blocks until the server stops; a clean Shutdown
	// makes it return http.ErrServerClosed, which is expected — not a real error.
	go func() {
		slog.Info("starting server", "addr", srv.Addr, "instance", cfg.InstanceID)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	// Block here until the OS asks us to stop. signal.Notify routes Ctrl+C (SIGINT) and
	// `kill`/orchestrator stop (SIGTERM) into this channel instead of letting them kill
	// the process instantly — that's what gives us the chance to shut down cleanly.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutdown signal received, shutting down gracefully")

	// Stop the HTTP server first: refuse new connections, let in-flight requests finish
	// (up to 10s). After this, no new tasks can enter via the API.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http server shutdown", "err", err)
	}
	slog.Info("http server stopped")

	// Returning now runs the deferred Stop()/Close() calls in LIFO order:
	//   scheduler.Stop() (stop submitting) → pool.Stop() (drain workers) →
	//   redis.Close() → mysql.Close().
	// That ordering IS the graceful drain.
}
