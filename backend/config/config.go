package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime settings, loaded from environment variables.
// More fields are added as later phases need them (worker count, rate limit, etc.).
type Config struct {
	Port       string
	InstanceID string

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	RedisAddr     string
	RedisPassword string

	// WorkerCount is how many worker goroutines the pool spawns = the system's
	// concurrency level. Defaults to 4 if WORKER_COUNT is unset.
	WorkerCount int

	// SchedulerQuantum is how many tasks each client may dispatch per DRR round
	// (its per-turn quota). Lower = stricter fairness; higher = more throughput per
	// turn. Defaults to 2.
	SchedulerQuantum int

	// WatchdogInterval is how often the watchdog scans for stuck tasks. Default 15s.
	WatchdogInterval time.Duration

	// WatchdogTimeout is how long a task may sit in "running" before the watchdog
	// treats it as hung/crashed and requeues it. Must be comfortably larger than the
	// longest real task so a merely-slow task isn't wrongly reclaimed. Default 60s.
	WatchdogTimeout time.Duration

	// RateLimitMax is how many API requests one client may make per RateLimitWindow
	// before getting 429s. Default 10.
	RateLimitMax int

	// RateLimitWindow is the sliding window over which RateLimitMax is counted.
	// Default 60s.
	RateLimitWindow time.Duration
}

// Load reads configuration from the environment, applying defaults for any
// missing values. In development it first loads a .env file if one is present;
// in production the host injects real env vars and the .env file is absent.
func Load() *Config {
	_ = godotenv.Load() // dev convenience; not an error if .env is missing

	return &Config{
		Port:       getEnv("PORT", "8080"),
		InstanceID: getEnv("INSTANCE_ID", "backend-1"),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "root"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "task_engine"),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		WorkerCount:      getEnvInt("WORKER_COUNT", 4),
		SchedulerQuantum: getEnvInt("SCHEDULER_QUANTUM", 2),

		WatchdogInterval: getEnvSeconds("WATCHDOG_INTERVAL_SECONDS", 15),
		WatchdogTimeout:  getEnvSeconds("WATCHDOG_TIMEOUT_SECONDS", 60),

		RateLimitMax:    getEnvInt("RATE_LIMIT_MAX", 50),
		RateLimitWindow: getEnvSeconds("RATE_LIMIT_WINDOW_SECONDS", 60),
	}
}

// getEnv returns the value of the env var named key, or fallback if it is
// unset or empty.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvInt is the integer version of getEnv: it parses the env var as an int,
// falling back if it's unset, empty, or not a valid number.
func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// getEnvSeconds reads an int env var as a number of SECONDS and returns it as a
// time.Duration (so config stays simple ints like WATCHDOG_TIMEOUT_SECONDS=10).
func getEnvSeconds(key string, fallbackSeconds int) time.Duration {
	return time.Duration(getEnvInt(key, fallbackSeconds)) * time.Second
}
