package config

import (
	"os"

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
