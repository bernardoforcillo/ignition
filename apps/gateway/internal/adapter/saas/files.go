package saas

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bernardoforcillo/drops/pg"

	"github.com/bernardoforcillo/ignition/go-packages/features"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

// fileColumns is the column list every file query selects, in scanFile's order.
const fileColumns = `id, workspace_id, name, content_type, size_bytes, status, object_key, created_by, created_at`

// fileRepo is core.FileRepository over the files table (see createFiles). Every query that takes a
// workspace id filters on it, so a file id of another workspace is simply not found.
type fileRepo struct{ db *pg.DB }

var _ core.FileRepository = (*fileRepo)(nil)

// Reserve inserts the pending file after checking the quota, under a per-workspace advisory lock:
// two uploads racing for the last bytes are serialized, in this replica or another, so the second
// sees the first's reservation and is refused. The lock is released when the transaction ends.
func (r *fileRepo) Reserve(ctx context.Context, f core.File, quota int64) error {
	return r.db.InTx(ctx, func(tx *pg.DB) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "files:"+f.WorkspaceID); err != nil {
			return fmt.Errorf("locking workspace files: %w", err)
		}
		if quota >= 0 {
			used, err := usedBytes(ctx, tx, f.WorkspaceID)
			if err != nil {
				return err
			}
			if used+f.SizeBytes > quota {
				return core.ErrQuotaExceeded
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO files (id, workspace_id, name, content_type, size_bytes, status, object_key, created_by, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			f.ID, f.WorkspaceID, f.Name, f.ContentType, f.SizeBytes, string(f.Status), f.ObjectKey, f.CreatedBy, f.CreatedAt); err != nil {
			return fmt.Errorf("inserting file: %w", err)
		}
		return nil
	})
}

func usedBytes(ctx context.Context, db *pg.DB, workspaceID string) (int64, error) {
	rows, err := db.Query(ctx, `
		SELECT COALESCE(SUM(size_bytes), 0)::bigint FROM files
		WHERE workspace_id = $1 AND status IN ('pending', 'ready')`, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("reading used storage: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var n int64
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			return 0, fmt.Errorf("scanning used storage: %w", err)
		}
	}
	return n, rows.Err()
}

func (r *fileRepo) UsedBytes(ctx context.Context, workspaceID string) (int64, error) {
	return usedBytes(ctx, r.db, workspaceID)
}

type rowScanner interface{ Scan(dest ...any) error }

func scanFile(row rowScanner) (core.File, error) {
	var f core.File
	var status string
	if err := row.Scan(&f.ID, &f.WorkspaceID, &f.Name, &f.ContentType, &f.SizeBytes, &status, &f.ObjectKey, &f.CreatedBy, &f.CreatedAt); err != nil {
		return core.File{}, fmt.Errorf("scanning file: %w", err)
	}
	f.Status = core.FileStatus(status)
	f.CreatedAt = f.CreatedAt.UTC()
	return f, nil
}

// queryFiles runs a query that selects fileColumns.
func (r *fileRepo) queryFiles(ctx context.Context, what, query string, args ...any) ([]core.File, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer func() { _ = rows.Close() }()
	var out []core.File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return out, nil
}

// one runs a query expected to return at most one file.
func (r *fileRepo) one(ctx context.Context, what, query string, args ...any) (core.File, error) {
	files, err := r.queryFiles(ctx, what, query, args...)
	if err != nil {
		return core.File{}, err
	}
	if len(files) == 0 {
		return core.File{}, core.ErrFileNotFound
	}
	return files[0], nil
}

func (r *fileRepo) Get(ctx context.Context, workspaceID, id string) (core.File, error) {
	return r.one(ctx, "reading file", `SELECT `+fileColumns+` FROM files
		WHERE workspace_id = $1 AND id = $2 AND status IN ('pending', 'ready')`, workspaceID, id)
}

func (r *fileRepo) MarkReady(ctx context.Context, workspaceID, id string) (core.File, error) {
	return r.one(ctx, "completing file", `UPDATE files SET status = 'ready', completed_at = COALESCE(completed_at, now())
		WHERE workspace_id = $1 AND id = $2 AND status IN ('pending', 'ready')
		RETURNING `+fileColumns, workspaceID, id)
}

// ListReady pages newest first by (created_at, id), a total order, so no file is skipped or shown
// twice however many share a timestamp.
func (r *fileRepo) ListReady(ctx context.Context, workspaceID string, after *core.FileCursor, limit int) ([]core.File, error) {
	if after == nil {
		return r.queryFiles(ctx, "listing files", `SELECT `+fileColumns+` FROM files
			WHERE workspace_id = $1 AND status = 'ready'
			ORDER BY created_at DESC, id DESC LIMIT $2`, workspaceID, limit)
	}
	return r.queryFiles(ctx, "listing files", `SELECT `+fileColumns+` FROM files
		WHERE workspace_id = $1 AND status = 'ready' AND (created_at, id) < ($2, $3)
		ORDER BY created_at DESC, id DESC LIMIT $4`, workspaceID, after.CreatedAt, after.ID, limit)
}

func (r *fileRepo) exec(ctx context.Context, what, query string, args ...any) (int64, error) {
	res, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", what, err)
	}
	return res.RowsAffected()
}

func (r *fileRepo) Delete(ctx context.Context, workspaceID, id string) error {
	n, err := r.exec(ctx, "deleting file", `DELETE FROM files
		WHERE workspace_id = $1 AND id = $2 AND status IN ('pending', 'ready')`, workspaceID, id)
	if err == nil && n == 0 {
		return core.ErrFileNotFound
	}
	return err
}

func (r *fileRepo) Abandon(ctx context.Context, workspaceID, id string) (bool, error) {
	n, err := r.exec(ctx, "abandoning file", `UPDATE files SET status = 'abandoned'
		WHERE workspace_id = $1 AND id = $2 AND status = 'pending'`, workspaceID, id)
	return n == 1, err
}

func (r *fileRepo) ListAbandoned(ctx context.Context, pendingBefore time.Time, limit int) ([]core.File, error) {
	return r.queryFiles(ctx, "listing abandoned files", `SELECT `+fileColumns+` FROM files
		WHERE status = 'abandoned' OR (status = 'pending' AND created_at < $1)
		ORDER BY created_at, id LIMIT $2`, pendingBefore, limit)
}

func (r *fileRepo) DeleteAbandoned(ctx context.Context, workspaceID, id string) error {
	_, err := r.exec(ctx, "deleting abandoned file", `DELETE FROM files
		WHERE workspace_id = $1 AND id = $2 AND status = 'abandoned'`, workspaceID, id)
	return err
}

// storageQuota is core.StorageQuota over the feature engine: the plan's storage.bytes limit.
type storageQuota struct{ engine *features.Engine }

var _ core.StorageQuota = storageQuota{}

// Limit gates on the feature (not entitled, switched off and a store outage are errors the
// transport maps) and then reads the limit the plan and its add-ons add up to. A limit of nil means
// the entitlement is unmetered, which is unlimited storage.
func (q storageQuota) Limit(ctx context.Context, workspaceID, userID string) (int64, error) {
	if err := q.engine.Require(ctx, features.StorageBytes, workspaceID, userID); err != nil {
		return 0, err
	}
	d := q.engine.Evaluate(ctx, features.StorageBytes, workspaceID, userID)
	if d.Entitlement == nil {
		return 0, errors.New("storage entitlement could not be resolved")
	}
	if d.Entitlement.Limit == nil {
		return -1, nil
	}
	return d.Entitlement.Limit.Max, nil
}
