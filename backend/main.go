package main

import (
	"log/slog"
	"os"

	"github.com/Himank2026/task-execution-engine/backend/config"
	"github.com/Himank2026/task-execution-engine/backend/database"
	"github.com/Himank2026/task-execution-engine/backend/logger"
	"github.com/Himank2026/task-execution-engine/backend/routes"
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

	r := routes.SetupRouter()

	addr := ":" + cfg.Port
	slog.Info("starting server", "addr", addr, "instance", cfg.InstanceID)
	if err := r.Run(addr); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
