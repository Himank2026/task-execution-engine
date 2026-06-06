package main

import (
	"log/slog"
	"os"

	"github.com/Himank2026/task-execution-engine/backend/config"
	"github.com/Himank2026/task-execution-engine/backend/database"
	"github.com/Himank2026/task-execution-engine/backend/logger"
	"github.com/Himank2026/task-execution-engine/backend/routes"
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

	// One shared TaskService instance for BOTH the HTTP layer and the worker pool,
	// so the API and the workers operate on the exact same business logic.
	taskService := services.NewTaskService(db)

	// Start the worker pool: N goroutines pulling pending tasks and running them.
	pool := worker.NewPool(taskService, cfg.InstanceID, cfg.WorkerCount)
	pool.Start()
	defer pool.Stop() // graceful: when main returns, let in-flight workers finish

	r := routes.SetupRouter(db, taskService)

	addr := ":" + cfg.Port
	slog.Info("starting server", "addr", addr, "instance", cfg.InstanceID)
	if err := r.Run(addr); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
