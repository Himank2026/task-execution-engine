package database

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Himank2026/task-execution-engine/backend/config"
)

// ConnectMySQL opens a GORM connection to MySQL using cfg, tunes the underlying
// connection pool, and verifies it with a ping. The returned *gorm.DB is safe
// for concurrent use by the whole worker pool.
func ConnectMySQL(cfg *config.Config) (*gorm.DB, error) {
	// DSN (Data Source Name): user:pass@tcp(host:port)/dbname?params
	//   parseTime=true  -> scan DATETIME/TIMESTAMP into Go time.Time
	//   charset=utf8mb4 -> full unicode
	//   loc=Local       -> interpret DB times in the local timezone
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=Local",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)

	gormDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		// Warn-level: log slow queries and errors, not every statement.
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	// GORM wraps the standard database/sql pool. Reach it via .DB() to tune the
	// pool and to ping — the same knobs we'd use with raw database/sql.
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(25)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	// Ping forces a real connection so startup fails immediately if MySQL is
	// unreachable, the database is missing, or credentials are wrong.
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	return gormDB, nil
}
