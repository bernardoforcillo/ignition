package database

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/bernardoforcillo/drops/pg"
)

func noop(context.Context, *pg.DB) error { return nil }

func ids(ms []Migration) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}

func TestPlan_OrdersByID(t *testing.T) {
	got, err := plan([]Migration{{ID: "0002_b", Up: noop}, {ID: "0001_a", Up: noop}})
	if err != nil {
		t.Fatal(err)
	}
	if g := ids(got); g[0] != "0001_a" || g[1] != "0002_b" {
		t.Errorf("got %v", g)
	}
}

func TestPlan_RejectsInvalidSets(t *testing.T) {
	tests := []struct {
		name string
		in   []Migration
		want error
	}{
		{"empty ID", []Migration{{Up: noop}}, ErrEmptyMigrationID},
		{"nil Up", []Migration{{ID: "0001"}}, ErrNilMigrationUp},
		{"duplicate ID", []Migration{{ID: "0001", Up: noop}, {ID: "0001", Up: noop}}, ErrDuplicateMigrationID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := plan(tt.in); !errors.Is(err, tt.want) {
				t.Errorf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestFromFS_LoadsOnlySQLFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/0002_b.sql": {Data: []byte("SELECT 2")},
		"migrations/0001_a.sql": {Data: []byte("SELECT 1")},
		"migrations/README.md":  {Data: []byte("ignored")},
		"migrations/sub/x.sql":  {Data: []byte("ignored")},
		"elsewhere/0009_z.sql":  {Data: []byte("ignored")},
	}
	got, err := FromFS(fsys, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if g := ids(got); len(g) != 2 || g[0] != "0001_a" || g[1] != "0002_b" {
		t.Errorf("got %v", g)
	}
}

func TestFromFS_MissingDirFails(t *testing.T) {
	if _, err := FromFS(fstest.MapFS{}, "nope"); err == nil {
		t.Fatal("want error")
	}
}
