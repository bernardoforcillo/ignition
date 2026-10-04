package database

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// ErrNoDSN is returned when a Config has no connection string.
var ErrNoDSN = errors.New("database: DSN is empty")

// Pool defaults: conservative enough for a small SaaS behind a managed
// Postgres whose connection limit is shared by several replicas.
const (
	defaultMaxOpenConns    = 10
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 30 * time.Minute
)

// Config holds everything Open needs. Zero pool values fall back to defaults.
type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// FromEnv builds a Config from DATABASE_URL and the optional
// DATABASE_MAX_OPEN_CONNS, DATABASE_MAX_IDLE_CONNS and
// DATABASE_CONN_MAX_LIFETIME (a Go duration such as "15m"). It is the only
// place this package reads the environment.
func FromEnv() (Config, error) {
	cfg := Config{DSN: os.Getenv("DATABASE_URL")}
	if cfg.DSN == "" {
		return Config{}, fmt.Errorf("DATABASE_URL: %w", ErrNoDSN)
	}
	var err error
	if cfg.MaxOpenConns, err = envInt("DATABASE_MAX_OPEN_CONNS"); err != nil {
		return Config{}, err
	}
	if cfg.MaxIdleConns, err = envInt("DATABASE_MAX_IDLE_CONNS"); err != nil {
		return Config{}, err
	}
	if v := os.Getenv("DATABASE_CONN_MAX_LIFETIME"); v != "" {
		if cfg.ConnMaxLifetime, err = time.ParseDuration(v); err != nil {
			return Config{}, fmt.Errorf("DATABASE_CONN_MAX_LIFETIME: %w", err)
		}
	}
	return cfg, nil
}

func envInt(key string) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s: want a non-negative integer, got %q", key, v)
	}
	return n, nil
}

// withDefaults returns c with zero pool values replaced by the defaults.
func (c Config) withDefaults() Config {
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = defaultMaxOpenConns
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = defaultMaxIdleConns
	}
	if c.ConnMaxLifetime == 0 {
		c.ConnMaxLifetime = defaultConnMaxLifetime
	}
	// database/sql silently caps idle at open; make that explicit.
	if c.MaxIdleConns > c.MaxOpenConns {
		c.MaxIdleConns = c.MaxOpenConns
	}
	return c
}
