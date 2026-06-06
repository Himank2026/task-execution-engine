package main

import (
	"log/slog"
	"os"

	"github.com/Himank2026/task-execution-engine/backend/config"
	"github.com/Himank2026/task-execution-engine/backend/database"
	"github.com/Himank2026/task-execution-engine/backend/logger"
	"github.com/Himank2026/task-execution-engine/backend/routes"
	"github.com/Himank2026/task-execution-engine/backend/scheduler"
	"github.com/Himank2026/task-execution-engine/backend/services"
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
		defer sqlDB.Close()
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
	defer rdb.Close()
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

	// Start the worker pool (execution): N goroutines running whatever they're given.
	pool := worker.NewPool(taskService, cfg.InstanceID, cfg.WorkerCount)
	pool.Start()
	defer pool.Stop() // runs second (LIFO): drain workers after scheduler stops

	// Start the scheduler (fairness): DRR pass that feeds the pool fairly per client.
	sched := scheduler.NewScheduler(taskService, pool, cfg.InstanceID, cfg.SchedulerQuantum)
	sched.Start()
	defer sched.Stop() // runs first (LIFO): stop submitting before the pool closes

	r := routes.SetupRouter(db, taskService)

	addr := ":" + cfg.Port
	slog.Info("starting server", "addr", addr, "instance", cfg.InstanceID)
	if err := r.Run(addr); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
