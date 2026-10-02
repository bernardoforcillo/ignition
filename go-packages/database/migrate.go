package database

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/bernardoforcillo/drops/pg"
)

// MigrationsTable records applied migration IDs.
const MigrationsTable = "schema_migrations"

// Sentinel errors for invalid migration sets, checked before touching the
// database so a bad set never half-applies.
var (
	ErrEmptyMigrationID     = errors.New("database: migration has an empty ID")
	ErrDuplicateMigrationID = errors.New("database: duplicate migration ID")
	ErrNilMigrationUp       = errors.New("database: migration has no Up")
)

// Migration is one forward-only schema change. IDs sort lexicographically
// into apply order, so zero-pad them ("0001_create_users").
type Migration struct {
	ID string
	Up func(ctx context.Context, tx *pg.DB) error
}

// SQL returns a Migration that executes a raw SQL script.
func SQL(id, script string) Migration {
	return Migration{ID: id, Up: func(ctx context.Context, tx *pg.DB) error {
		_, err := tx.Exec(ctx, script)
		return err
	}}
}

// FromFS builds migrations from the "<id>.sql" files directly under dir
// (typically an embed.FS), e.g. "0001_create_users.sql". Other files and
// subdirectories are ignored.
func FromFS(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("database: read migrations dir %q: %w", dir, err)
	}
	var migs []Migration
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".sql")
		if e.IsDir() || !ok {
			continue
		}
		body, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("database: read migration %q: %w", e.Name(), err)
		}
		migs = append(migs, SQL(id, string(body)))
	}
	return migs, nil
}

// plan validates migs and returns them in apply order.
func plan(migs []Migration) ([]Migration, error) {
	out := append([]Migration(nil), migs...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	for i, m := range out {
		switch {
		case m.ID == "":
			return nil, ErrEmptyMigrationID
		case m.Up == nil:
			return nil, fmt.Errorf("%w: %s", ErrNilMigrationUp, m.ID)
		case i > 0 && out[i-1].ID == m.ID:
			return nil, fmt.Errorf("%w: %s", ErrDuplicateMigrationID, m.ID)
		}
	}
	return out, nil
}

// Migrate applies the migrations not yet recorded in schema_migrations, in
// ID order, each in its own transaction. Re-running is a no-op, and
// concurrent callers (one per replica on a rolling deploy) are serialised
// by drops' Postgres advisory lock, so the first applies and the rest wait.
func Migrate(ctx context.Context, db *DB, migs ...Migration) error {
	ordered, err := plan(migs)
	if err != nil {
		return err
	}
	m := pg.NewMigrator(db.DB).WithTable(MigrationsTable)
	for _, mig := range ordered {
		m.Add(pg.Migration{Version: mig.ID, Name: mig.ID, Up: mig.Up})
	}
	if err := m.Up(ctx); err != nil {
		return fmt.Errorf("database: migrate: %w", err)
	}
	return nil
}
