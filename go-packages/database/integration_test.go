package database_test

import (
	"context"
	"testing"

	"github.com/bernardoforcillo/drops/pg"
	"github.com/bernardoforcillo/ignition/go-packages/database"
	"github.com/bernardoforcillo/ignition/go-packages/database/dbtest"
)

func TestMigrate_IsIdempotentAndRecordsHistory(t *testing.T) {
	db := dbtest.Open(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = db.Exec(ctx, `DROP TABLE IF EXISTS database_it, schema_migrations`)
	})

	migs := []database.Migration{
		database.SQL("0001_create", `CREATE TABLE database_it (id int PRIMARY KEY)`),
		database.SQL("0002_insert", `INSERT INTO database_it VALUES (1)`),
	}
	for range 2 {
		if err := database.Migrate(ctx, db, migs...); err != nil {
			t.Fatalf("Migrate: %v", err)
		}
	}
}

func TestWithTx_RollsBackOnError(t *testing.T) {
	db := dbtest.Open(t)
	ctx := context.Background()
	t.Cleanup(func() { _, _ = db.Exec(ctx, `DROP TABLE IF EXISTS database_tx`) })
	if _, err := db.Exec(ctx, `CREATE TABLE database_tx (id int)`); err != nil {
		t.Fatal(err)
	}

	errBoom := context.Canceled
	err := db.WithTx(ctx, func(tx *pg.DB) error {
		if _, err := tx.Exec(ctx, `INSERT INTO database_tx VALUES (1)`); err != nil {
			return err
		}
		return errBoom
	})
	if err != errBoom {
		t.Fatalf("got %v, want %v", err, errBoom)
	}
	rows, err := db.Query(ctx, `SELECT id FROM database_tx`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Error("row survived rollback")
	}
}
