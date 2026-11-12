package app

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config — параметры процесса. Имена переменных зафиксированы курсом.
type Config struct {
	HTTPAddr              string
	LogLevel              slog.Level
	ShutdownTimeout       time.Duration
	HTTPReadTimeout       time.Duration
	HTTPReadHeaderTimeout time.Duration
	HTTPWriteTimeout      time.Duration
	HTTPIdleTimeout       time.Duration

	DatabaseURL             string
	DatabaseMaxConns        int32
	DatabaseMinConns        int32
	DatabaseMaxConnLifetime time.Duration
	DatabaseConnectTimeout  time.Duration
	DatabaseQueryTimeout    time.Duration
	IdempotencyTTL          time.Duration
}

// ParseEnv читает конфигурацию из окружения. Пустая обязательная переменная
// и нулевой таймаут — ошибка старта, а не молчаливое значение по умолчанию.
func ParseEnv() (Config, error) {
	var cfg Config
	var err error

	if cfg.HTTPAddr, err = required("HTTP_ADDR"); err != nil {
		return Config{}, err
	}
	if cfg.LogLevel, err = logLevel("LOG_LEVEL"); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = duration("SHUTDOWN_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTPReadTimeout, err = duration("HTTP_READ_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTPReadHeaderTimeout, err = duration("HTTP_READ_HEADER_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTPWriteTimeout, err = duration("HTTP_WRITE_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTPIdleTimeout, err = duration("HTTP_IDLE_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseURL, err = required("DATABASE_URL"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseMaxConns, err = positiveInt32("DATABASE_MAX_CONNS"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseMinConns, err = nonNegativeInt32("DATABASE_MIN_CONNS"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseMinConns > cfg.DatabaseMaxConns {
		return Config{}, fmt.Errorf("DATABASE_MIN_CONNS must not exceed DATABASE_MAX_CONNS")
	}
	if cfg.DatabaseMaxConnLifetime, err = duration("DATABASE_MAX_CONN_LIFETIME"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseConnectTimeout, err = duration("DATABASE_CONNECT_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseQueryTimeout, err = duration("DATABASE_QUERY_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.IdempotencyTTL, err = duration("IDEMPOTENCY_TTL"); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func required(key string) (string, error) {
	value, ok := os.LookupEnv(key)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}
	return value, nil
}

func duration(key string) (time.Duration, error) {
	raw, err := required(key)
	if err != nil {
		return 0, err
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("environment variable %s must be a duration: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("environment variable %s must be greater than 0", key)
	}
	return value, nil
}

func positiveInt32(key string) (int32, error) {
	value, err := parseInt32(key)
	if err != nil {
		return 0, err
	}
	if value < 1 {
		return 0, fmt.Errorf("environment variable %s must be greater than 0", key)
	}
	return value, nil
}

func nonNegativeInt32(key string) (int32, error) {
	value, err := parseInt32(key)
	if err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, fmt.Errorf("environment variable %s must be greater than or equal to 0", key)
	}
	return value, nil
}

func parseInt32(key string) (int32, error) {
	raw, err := required(key)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("environment variable %s must be an integer: %w", key, err)
	}
	return int32(value), nil
}

func logLevel(key string) (slog.Level, error) {
	raw, err := required(key)
	if err != nil {
		return 0, err
	}
	switch strings.ToLower(raw) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("environment variable %s must be one of debug, info, warn, error", key)
	}
}
