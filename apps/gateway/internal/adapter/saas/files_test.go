package saas

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bernardoforcillo/drops/pg"

	"github.com/bernardoforcillo/ignition/go-packages/database"
	"github.com/bernardoforcillo/ignition/go-packages/database/dbtest"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

// scratchSchemaDB opens the test database on a private schema with every migration applied, so
// these tests neither see nor drop what other packages' tests keep in the shared database.
func scratchSchemaDB(t *testing.T) *pg.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("needs external service")
	}
	admin := dbtest.Open(t) // skips without TEST_DATABASE_URL
	var b [6]byte
	_, _ = rand.Read(b[:])
	schema := "saas_files_" + hex.EncodeToString(b[:])
	ctx := context.Background()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })

	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := database.Open(ctx, database.Config{DSN: u.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db, Migrations()...); err != nil {
		t.Fatal(err)
	}
	return db.DB
}

var fileT0 = time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)

func newFile(ws, id string, size int64, at time.Time) core.File {
	return core.File{
		ID: id, WorkspaceID: ws, Name: id + ".txt", ContentType: "text/plain", SizeBytes: size,
		Status: core.FilePending, ObjectKey: "workspaces/" + ws + "/" + id, CreatedBy: "u1", CreatedAt: at,
	}
}

func TestFileRepo_LifecycleAndTenantIsolation(t *testing.T) {
	db := scratchSchemaDB(t)
	repo := &fileRepo{db: db}
	ctx := context.Background()

	f := newFile("ws-a", "f1", 10, fileT0)
	if err := repo.Reserve(ctx, f, -1); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "ws-a", "f1")
	if err != nil || got != f {
		t.Fatalf("Get = %+v, %v, want %+v", got, err, f)
	}

	// Another workspace can neither see nor change it, with the right id in hand.
	if _, err := repo.Get(ctx, "ws-b", "f1"); !errors.Is(err, core.ErrFileNotFound) {
		t.Errorf("Get across workspaces = %v, want ErrFileNotFound", err)
	}
	if _, err := repo.MarkReady(ctx, "ws-b", "f1"); !errors.Is(err, core.ErrFileNotFound) {
		t.Errorf("MarkReady across workspaces = %v", err)
	}
	if err := repo.Delete(ctx, "ws-b", "f1"); !errors.Is(err, core.ErrFileNotFound) {
		t.Errorf("Delete across workspaces = %v", err)
	}
	if ok, err := repo.Abandon(ctx, "ws-b", "f1"); ok || err != nil {
		t.Errorf("Abandon across workspaces = %v, %v", ok, err)
	}
	if used, _ := repo.UsedBytes(ctx, "ws-b"); used != 0 {
		t.Errorf("ws-b used = %d, want 0", used)
	}

	ready, err := repo.MarkReady(ctx, "ws-a", "f1")
	if err != nil || ready.Status != core.FileReady {
		t.Fatalf("MarkReady = %+v, %v", ready, err)
	}
	if again, err := repo.MarkReady(ctx, "ws-a", "f1"); err != nil || again.Status != core.FileReady {
		t.Errorf("MarkReady twice = %+v, %v", again, err)
	}
	if ok, _ := repo.Abandon(ctx, "ws-a", "f1"); ok {
		t.Error("a ready file must not be abandoned")
	}
	if list, _ := repo.ListReady(ctx, "ws-b", nil, 10); len(list) != 0 {
		t.Errorf("ws-b lists %d files of ws-a", len(list))
	}

	if err := repo.Delete(ctx, "ws-a", "f1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, "ws-a", "f1"); !errors.Is(err, core.ErrFileNotFound) {
		t.Errorf("second Delete = %v, want ErrFileNotFound", err)
	}
}

func TestFileRepo_ObjectKeyIsUnique(t *testing.T) {
	repo := &fileRepo{db: scratchSchemaDB(t)}
	ctx := context.Background()
	a := newFile("ws-a", "f1", 1, fileT0)
	b := newFile("ws-a", "f2", 1, fileT0)
	b.ObjectKey = a.ObjectKey
	if err := repo.Reserve(ctx, a, -1); err != nil {
		t.Fatal(err)
	}
	if err := repo.Reserve(ctx, b, -1); err == nil {
		t.Fatal("two files must never share an object")
	}
}

func TestFileRepo_ListReadyKeysetPagingIsStableWithEqualTimestamps(t *testing.T) {
	repo := &fileRepo{db: scratchSchemaDB(t)}
	ctx := context.Background()
	// Six files, three timestamps, two files each: ties must not skip or repeat a row.
	for i, id := range []string{"a", "b", "c", "d", "e", "f"} {
		f := newFile("ws-a", id, 1, fileT0.Add(time.Duration(i/2)*time.Second))
		if err := repo.Reserve(ctx, f, -1); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.MarkReady(ctx, "ws-a", id); err != nil {
			t.Fatal(err)
		}
	}
	// A pending and an abandoned file never show.
	if err := repo.Reserve(ctx, newFile("ws-a", "p", 1, fileT0), -1); err != nil {
		t.Fatal(err)
	}

	var got []string
	var after *core.FileCursor
	for {
		page, err := repo.ListReady(ctx, "ws-a", after, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range page {
			got = append(got, f.ID)
		}
		if len(page) < 2 {
			break
		}
		last := page[len(page)-1]
		after = &core.FileCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	want := []string{"f", "e", "d", "c", "b", "a"}
	if len(got) != len(want) {
		t.Fatalf("listed %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("listed %v, want %v", got, want)
		}
	}
}

func TestFileRepo_ReserveEnforcesTheQuotaAtomicallyAcrossConcurrentUploads(t *testing.T) {
	repo := &fileRepo{db: scratchSchemaDB(t)}
	ctx := context.Background()

	// 20 uploads of 10 bytes race for a 100-byte quota: exactly 10 may win, whatever the interleaving.
	var ok, refused atomic.Int32
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f := newFile("ws-a", "f"+string(rune('a'+i)), 10, fileT0.Add(time.Duration(i)*time.Millisecond))
			switch err := repo.Reserve(ctx, f, 100); {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, core.ErrQuotaExceeded):
				refused.Add(1)
			default:
				t.Errorf("Reserve: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 10 || refused.Load() != 10 {
		t.Fatalf("accepted %d, refused %d; want 10 and 10", ok.Load(), refused.Load())
	}
	if used, _ := repo.UsedBytes(ctx, "ws-a"); used != 100 {
		t.Errorf("used = %d, want 100", used)
	}
	// Another workspace is not affected by ws-a being full.
	if err := repo.Reserve(ctx, newFile("ws-b", "g", 100, fileT0), 100); err != nil {
		t.Errorf("ws-b refused because ws-a is full: %v", err)
	}
}

func TestFileRepo_AbandonedFilesAreCleanedUpAndFreeTheirBytes(t *testing.T) {
	repo := &fileRepo{db: scratchSchemaDB(t)}
	ctx := context.Background()
	old, fresh := fileT0.Add(-2*time.Hour), fileT0.Add(-time.Minute)
	for _, f := range []core.File{newFile("ws-a", "old", 10, old), newFile("ws-a", "fresh", 20, fresh), newFile("ws-a", "done", 40, old)} {
		if err := repo.Reserve(ctx, f, -1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.MarkReady(ctx, "ws-a", "done"); err != nil {
		t.Fatal(err)
	}

	list, err := repo.ListAbandoned(ctx, fileT0.Add(-time.Hour), 10)
	if err != nil || len(list) != 1 || list[0].ID != "old" {
		t.Fatalf("ListAbandoned = %+v, %v, want only the stale pending file", list, err)
	}
	if ok, err := repo.Abandon(ctx, "ws-a", "old"); !ok || err != nil {
		t.Fatalf("Abandon = %v, %v", ok, err)
	}
	if used, _ := repo.UsedBytes(ctx, "ws-a"); used != 60 {
		t.Errorf("used = %d, want 60: an abandoned file no longer counts", used)
	}
	if _, err := repo.Get(ctx, "ws-a", "old"); !errors.Is(err, core.ErrFileNotFound) {
		t.Errorf("an abandoned file must be invisible: %v", err)
	}
	// DeleteAbandoned removes only abandoned rows.
	if err := repo.DeleteAbandoned(ctx, "ws-a", "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, "ws-a", "done"); err != nil {
		t.Errorf("DeleteAbandoned removed a ready file: %v", err)
	}
	if err := repo.DeleteAbandoned(ctx, "ws-a", "old"); err != nil {
		t.Fatal(err)
	}
	if list, _ := repo.ListAbandoned(ctx, fileT0, 10); len(list) != 1 || list[0].ID != "fresh" {
		t.Errorf("after cleanup ListAbandoned = %+v, want only fresh (now older than the cutoff)", list)
	}
}

func TestEraseWorkspace_AbandonsItsFilesForTheCleanupJob(t *testing.T) {
	db := scratchSchemaDB(t)
	repo := &fileRepo{db: db}
	ctx := context.Background()
	const erased = "11111111-1111-1111-1111-111111111111"
	for _, f := range []core.File{newFile(erased, "x", 5, fileT0), newFile("ws-keep", "y", 5, fileT0)} {
		if err := repo.Reserve(ctx, f, -1); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.MarkReady(ctx, f.WorkspaceID, f.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := (&directory{db: db}).EraseWorkspace(ctx, erased); err != nil {
		t.Fatal(err)
	}
	list, err := repo.ListAbandoned(ctx, fileT0.Add(-time.Hour), 10)
	if err != nil || len(list) != 1 || list[0].ID != "x" {
		t.Fatalf("abandoned after erase = %+v, %v, want the erased workspace's file only", list, err)
	}
	if _, err := repo.Get(ctx, "ws-keep", "y"); err != nil {
		t.Errorf("another workspace's file was touched: %v", err)
	}
}
