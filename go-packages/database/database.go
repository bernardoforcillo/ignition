// Package database is a thin Postgres layer over drops: a pooled connection
// with sane defaults, an ordered idempotent migration runner and a
// transaction helper. It owns no schema; each service brings its own
// migrations.
package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bernardoforcillo/drops/pg"
	"github.com/bernardoforcillo/drops/stdlib"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

// DB is a drops Postgres handle. It embeds *pg.DB so the full drops query
// builder, Ping, Close and InTx are available without re-wrapping.
type DB struct {
	*pg.DB
}

// Open connects with the pool settings from cfg and verifies the server is
// reachable, so a bad DSN fails at startup rather than on the first request.
func Open(ctx context.Context, cfg Config) (*DB, error) {
	if cfg.DSN == "" {
		return nil, ErrNoDSN
	}
	cfg = cfg.withDefaults()

	sqlDB, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("database: open: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	db := &DB{DB: pg.New(stdlib.New(sqlDB))}
	if err := db.Ping(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("database: ping: %w", err)
	}
	return db, nil
}

// WithTx runs fn in a transaction: commit on nil, rollback on error or
// panic. The tx handle has the same query API as DB, so repositories can
// accept either.
func (db *DB) WithTx(ctx context.Context, fn func(tx *pg.DB) error) error {
	return db.InTx(ctx, fn)
}
