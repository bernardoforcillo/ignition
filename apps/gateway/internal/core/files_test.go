package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

var filesNow = time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)

// fakeFileRepo is an in-memory FileRepository with the same contract as the SQL one.
type fakeFileRepo struct {
	files map[string]File // key: workspace + "/" + id
	// failDelete makes Delete fail once.
	failDelete error
}

func newFakeFileRepo() *fakeFileRepo { return &fakeFileRepo{files: map[string]File{}} }

func k(ws, id string) string { return ws + "/" + id }

func (r *fakeFileRepo) used(ws string) int64 {
	var n int64
	for _, f := range r.files {
		if f.WorkspaceID == ws && f.Status != FileAbandoned {
			n += f.SizeBytes
		}
	}
	return n
}

func (r *fakeFileRepo) Reserve(_ context.Context, f File, quota int64) error {
	if quota >= 0 && r.used(f.WorkspaceID)+f.SizeBytes > quota {
		return ErrQuotaExceeded
	}
	r.files[k(f.WorkspaceID, f.ID)] = f
	return nil
}

func (r *fakeFileRepo) Get(_ context.Context, ws, id string) (File, error) {
	f, ok := r.files[k(ws, id)]
	if !ok || f.Status == FileAbandoned {
		return File{}, ErrFileNotFound
	}
	return f, nil
}

func (r *fakeFileRepo) MarkReady(_ context.Context, ws, id string) (File, error) {
	f, err := r.Get(context.Background(), ws, id)
	if err != nil {
		return File{}, err
	}
	f.Status = FileReady
	r.files[k(ws, id)] = f
	return f, nil
}

func (r *fakeFileRepo) ListReady(_ context.Context, ws string, after *FileCursor, limit int) ([]File, error) {
	var out []File
	for _, f := range r.files {
		if f.WorkspaceID != ws || f.Status != FileReady {
			continue
		}
		if after != nil && !(f.CreatedAt.Before(after.CreatedAt) || (f.CreatedAt.Equal(after.CreatedAt) && f.ID < after.ID)) {
			continue
		}
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b File) int {
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return b.CreatedAt.Compare(a.CreatedAt)
		}
		return strings.Compare(b.ID, a.ID)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *fakeFileRepo) UsedBytes(_ context.Context, ws string) (int64, error) { return r.used(ws), nil }

func (r *fakeFileRepo) Delete(_ context.Context, ws, id string) error {
	if r.failDelete != nil {
		err := r.failDelete
		r.failDelete = nil
		return err
	}
	if _, ok := r.files[k(ws, id)]; !ok {
		return ErrFileNotFound
	}
	delete(r.files, k(ws, id))
	return nil
}

func (r *fakeFileRepo) Abandon(_ context.Context, ws, id string) (bool, error) {
	f, ok := r.files[k(ws, id)]
	if !ok || f.Status != FilePending {
		return false, nil
	}
	f.Status = FileAbandoned
	r.files[k(ws, id)] = f
	return true, nil
}

func (r *fakeFileRepo) ListAbandoned(_ context.Context, before time.Time, limit int) ([]File, error) {
	var out []File
	for _, f := range r.files {
		if f.Status == FileAbandoned || (f.Status == FilePending && f.CreatedAt.Before(before)) {
			out = append(out, f)
		}
	}
	slices.SortFunc(out, func(a, b File) int { return strings.Compare(a.ID, b.ID) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *fakeFileRepo) DeleteAbandoned(_ context.Context, ws, id string) error {
	delete(r.files, k(ws, id))
	return nil
}

type fakeObjects struct {
	objects map[string]ObjectInfo
	signErr error
	// failDelete makes Delete fail for these keys.
	failDelete map[string]bool
	signed     []string
	lastName   string
}

func newFakeObjects() *fakeObjects {
	return &fakeObjects{objects: map[string]ObjectInfo{}, failDelete: map[string]bool{}}
}

func (o *fakeObjects) SignUpload(_ context.Context, key, ct string, size int64, ttl time.Duration) (SignedRequest, error) {
	if o.signErr != nil {
		return SignedRequest{}, o.signErr
	}
	o.signed = append(o.signed, key)
	return SignedRequest{URL: "https://storage.test/" + key, Method: "PUT",
		Headers: map[string]string{"Content-Type": ct}, ExpiresAt: filesNow.Add(ttl)}, nil
}

func (o *fakeObjects) SignDownload(_ context.Context, key, name string, ttl time.Duration) (SignedRequest, error) {
	o.lastName = name
	return SignedRequest{URL: "https://storage.test/get/" + key, Method: "GET", ExpiresAt: filesNow.Add(ttl)}, nil
}

func (o *fakeObjects) Stat(_ context.Context, key string) (ObjectInfo, error) {
	info, ok := o.objects[key]
	if !ok {
		return ObjectInfo{}, ErrObjectNotFound
	}
	return info, nil
}

func (o *fakeObjects) Delete(_ context.Context, key string) error {
	if o.failDelete[key] {
		return errors.New("storage down")
	}
	delete(o.objects, key)
	return nil
}

type fakeQuota struct {
	limit int64
	err   error
}

func (q fakeQuota) Limit(context.Context, string, string) (int64, error) { return q.limit, q.err }

type filesFixture struct {
	svc     *Files
	repo    *fakeFileRepo
	objects *fakeObjects
	ids     int
}

func newFilesFixture(quota fakeQuota) *filesFixture {
	f := &filesFixture{repo: newFakeFileRepo(), objects: newFakeObjects()}
	limits := FileLimits{
		MaxFileBytes: 1000, AllowedTypes: []string{"text/plain", "image/png"},
		UploadTTL: 10 * time.Minute, DownloadTTL: 5 * time.Minute, PendingTTL: time.Hour,
	}
	f.svc = NewFiles(f.repo, f.objects, quota, limits,
		func() time.Time { return filesNow },
		func() string { f.ids++; return fmt.Sprintf("id-%02d", f.ids) })
	return f
}

// upload runs the whole happy path for one file: CreateUpload, the client's PUT, CompleteUpload.
func (f *filesFixture) upload(t *testing.T, ws string, size int64) File {
	t.Helper()
	up, err := f.svc.CreateUpload(context.Background(), ws, "u1", NewFile{Name: "a.txt", ContentType: "text/plain", SizeBytes: size})
	if err != nil {
		t.Fatal(err)
	}
	f.objects.objects[up.File.ObjectKey] = ObjectInfo{Size: size, ContentType: "text/plain"}
	done, err := f.svc.CompleteUpload(context.Background(), ws, up.File.ID)
	if err != nil {
		t.Fatal(err)
	}
	return done
}

func TestCreateUpload_KeyIsGeneratedFromWorkspaceAndIDOnly(t *testing.T) {
	f := newFilesFixture(fakeQuota{limit: -1})
	up, err := f.svc.CreateUpload(context.Background(), "ws-1", "u1",
		NewFile{Name: "../../../etc/passwd", ContentType: "text/plain", SizeBytes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if up.File.ObjectKey != "workspaces/ws-1/id-01" {
		t.Errorf("key = %q, want workspaces/<workspace>/<generated id>", up.File.ObjectKey)
	}
	if up.File.Name != "../../../etc/passwd" || up.File.Status != FilePending || up.File.CreatedBy != "u1" {
		t.Errorf("file = %+v", up.File)
	}
	if up.Request.Method != "PUT" || up.Request.Headers["Content-Type"] != "text/plain" || !up.Request.ExpiresAt.Equal(filesNow.Add(10*time.Minute)) {
		t.Errorf("request = %+v", up.Request)
	}
}

func TestCreateUpload_Validation(t *testing.T) {
	long := strings.Repeat("n", 256)
	tests := []struct {
		name string
		in   NewFile
		want error
	}{
		{"type not on the allow-list", NewFile{Name: "a.html", ContentType: "text/html", SizeBytes: 5}, ErrFileTypeNotAllowed},
		{"parameters do not smuggle a type", NewFile{Name: "a", ContentType: `text/html; x="text/plain"`, SizeBytes: 5}, ErrFileTypeNotAllowed},
		{"garbage content type", NewFile{Name: "a", ContentType: "not a type", SizeBytes: 5}, ErrFileInvalid},
		{"empty content type", NewFile{Name: "a", SizeBytes: 5}, ErrFileInvalid},
		{"too large", NewFile{Name: "a", ContentType: "text/plain", SizeBytes: 1001}, ErrFileTooLarge},
		{"zero size", NewFile{Name: "a", ContentType: "text/plain"}, ErrFileInvalid},
		{"negative size", NewFile{Name: "a", ContentType: "text/plain", SizeBytes: -1}, ErrFileInvalid},
		{"empty name", NewFile{Name: "  ", ContentType: "text/plain", SizeBytes: 5}, ErrFileInvalid},
		{"name with a control character", NewFile{Name: "a\r\nb", ContentType: "text/plain", SizeBytes: 5}, ErrFileInvalid},
		{"name too long", NewFile{Name: long, ContentType: "text/plain", SizeBytes: 5}, ErrFileInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFilesFixture(fakeQuota{limit: -1})
			_, err := f.svc.CreateUpload(context.Background(), "ws-1", "u1", tc.in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if len(f.repo.files) != 0 || len(f.objects.signed) != 0 {
				t.Error("a refused upload must reserve and sign nothing")
			}
		})
	}
}

func TestCreateUpload_NormalizesTheContentTypeItSigns(t *testing.T) {
	f := newFilesFixture(fakeQuota{limit: -1})
	up, err := f.svc.CreateUpload(context.Background(), "ws-1", "u1",
		NewFile{Name: "a.txt", ContentType: "Text/Plain; charset=UTF-8", SizeBytes: 5})
	if err != nil {
		t.Fatal(err)
	}
	if up.File.ContentType != "text/plain" || up.Request.Headers["Content-Type"] != "text/plain" {
		t.Errorf("content type = %q / %q, want text/plain", up.File.ContentType, up.Request.Headers["Content-Type"])
	}
}

func TestCreateUpload_QuotaCountsPendingAndReadyBytesPerWorkspace(t *testing.T) {
	f := newFilesFixture(fakeQuota{limit: 100})
	ctx := context.Background()
	f.upload(t, "ws-1", 60)
	if _, err := f.svc.CreateUpload(ctx, "ws-1", "u1", NewFile{Name: "b", ContentType: "text/plain", SizeBytes: 41}); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("41 more bytes over a 100 quota with 60 used: err = %v, want ErrQuotaExceeded", err)
	}
	// A reservation (pending) holds its bytes, so two uploads cannot both fit the last 40.
	if _, err := f.svc.CreateUpload(ctx, "ws-1", "u1", NewFile{Name: "c", ContentType: "text/plain", SizeBytes: 40}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateUpload(ctx, "ws-1", "u1", NewFile{Name: "d", ContentType: "text/plain", SizeBytes: 1}); !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("a pending reservation must count: err = %v", err)
	}
	// Another workspace has its own budget.
	if _, err := f.svc.CreateUpload(ctx, "ws-2", "u1", NewFile{Name: "e", ContentType: "text/plain", SizeBytes: 100}); err != nil {
		t.Errorf("tenant isolation: ws-2 refused because of ws-1's usage: %v", err)
	}
}

func TestCreateUpload_FailsClosedWhenTheQuotaCannotBeResolved(t *testing.T) {
	boom := errors.New("features store down")
	f := newFilesFixture(fakeQuota{err: boom})
	_, err := f.svc.CreateUpload(context.Background(), "ws-1", "u1", NewFile{Name: "a", ContentType: "text/plain", SizeBytes: 5})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the quota error", err)
	}
	if len(f.repo.files) != 0 {
		t.Error("nothing may be reserved when the quota is unknown")
	}
}

func TestCreateUpload_ReleasesTheReservationWhenSigningFails(t *testing.T) {
	f := newFilesFixture(fakeQuota{limit: 100})
	f.objects.signErr = errors.New("kms down")
	if _, err := f.svc.CreateUpload(context.Background(), "ws-1", "u1", NewFile{Name: "a", ContentType: "text/plain", SizeBytes: 90}); err == nil {
		t.Fatal("want the signing error")
	}
	if f.repo.used("ws-1") != 0 {
		t.Error("a failed upload must not keep its bytes reserved")
	}
}

func TestCompleteUpload(t *testing.T) {
	ctx := context.Background()
	create := func(f *filesFixture) File {
		up, err := f.svc.CreateUpload(ctx, "ws-1", "u1", NewFile{Name: "a.txt", ContentType: "text/plain", SizeBytes: 10})
		if err != nil {
			t.Fatal(err)
		}
		return up.File
	}

	t.Run("marks the file ready when the object matches, and is idempotent", func(t *testing.T) {
		f := newFilesFixture(fakeQuota{limit: -1})
		file := create(f)
		f.objects.objects[file.ObjectKey] = ObjectInfo{Size: 10, ContentType: "text/plain; charset=utf-8"}
		done, err := f.svc.CompleteUpload(ctx, "ws-1", file.ID)
		if err != nil || done.Status != FileReady {
			t.Fatalf("complete = %+v, %v", done, err)
		}
		if again, err := f.svc.CompleteUpload(ctx, "ws-1", file.ID); err != nil || again.Status != FileReady {
			t.Errorf("second complete = %+v, %v", again, err)
		}
	})
	t.Run("no object yet", func(t *testing.T) {
		f := newFilesFixture(fakeQuota{limit: -1})
		file := create(f)
		if _, err := f.svc.CompleteUpload(ctx, "ws-1", file.ID); !errors.Is(err, ErrUploadMissing) {
			t.Fatalf("err = %v, want ErrUploadMissing", err)
		}
		if got, _ := f.repo.Get(ctx, "ws-1", file.ID); got.Status != FilePending {
			t.Error("the file must stay pending so the client can retry")
		}
	})
	for name, obj := range map[string]ObjectInfo{
		"size differs": {Size: 11, ContentType: "text/plain"},
		"type differs": {Size: 10, ContentType: "text/html"},
	} {
		t.Run(name+": object and record are removed", func(t *testing.T) {
			f := newFilesFixture(fakeQuota{limit: -1})
			file := create(f)
			f.objects.objects[file.ObjectKey] = obj
			if _, err := f.svc.CompleteUpload(ctx, "ws-1", file.ID); !errors.Is(err, ErrUploadMismatch) {
				t.Fatalf("err = %v, want ErrUploadMismatch", err)
			}
			if _, ok := f.objects.objects[file.ObjectKey]; ok {
				t.Error("the mismatching object must be deleted")
			}
			if f.repo.used("ws-1") != 0 {
				t.Error("a mismatching upload must not keep occupying quota")
			}
		})
	}
	t.Run("another workspace cannot complete it", func(t *testing.T) {
		f := newFilesFixture(fakeQuota{limit: -1})
		file := create(f)
		f.objects.objects[file.ObjectKey] = ObjectInfo{Size: 10, ContentType: "text/plain"}
		if _, err := f.svc.CompleteUpload(ctx, "ws-2", file.ID); !errors.Is(err, ErrFileNotFound) {
			t.Fatalf("err = %v, want ErrFileNotFound", err)
		}
	})
}

func TestListFiles_KeysetPagingNewestFirst(t *testing.T) {
	f := newFilesFixture(fakeQuota{limit: 1000})
	ctx := context.Background()
	// Five ready files created one second apart, plus one in another workspace and one pending.
	var ids []string
	orig := filesNow
	t.Cleanup(func() { filesNow = orig })
	for range 5 {
		filesNow = filesNow.Add(time.Second)
		ids = append(ids, f.upload(t, "ws-1", 10).ID)
	}
	f.upload(t, "ws-2", 10)
	if _, err := f.svc.CreateUpload(ctx, "ws-1", "u1", NewFile{Name: "p", ContentType: "text/plain", SizeBytes: 7}); err != nil {
		t.Fatal(err)
	}

	var got []string
	token := ""
	pages := 0
	for {
		page, err := f.svc.ListFiles(ctx, "ws-1", "u1", 2, token)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, file := range page.Files {
			got = append(got, file.ID)
		}
		if page.UsedBytes != 57 || page.Quota == nil || *page.Quota != 1000 {
			t.Fatalf("figures = used %d quota %v, want 57 of 1000 (pending counts, other workspaces do not)", page.UsedBytes, page.Quota)
		}
		if token = page.NextToken; token == "" {
			break
		}
	}
	want := []string{ids[4], ids[3], ids[2], ids[1], ids[0]}
	if !slices.Equal(got, want) || pages != 3 {
		t.Errorf("listed %v in %d pages, want %v in 3 (newest first, ready only, this workspace only)", got, pages, want)
	}
}

func TestListFiles_Edges(t *testing.T) {
	ctx := context.Background()
	f := newFilesFixture(fakeQuota{limit: -1})
	page, err := f.svc.ListFiles(ctx, "ws-1", "u1", 0, "")
	if err != nil || len(page.Files) != 0 || page.NextToken != "" || page.Quota != nil {
		t.Errorf("empty workspace = %+v, %v (an unlimited plan has no quota)", page, err)
	}
	for _, token := range []string{"!!!", "bm90LWEtY3Vyc29y", "MTIzLg"} {
		if _, err := f.svc.ListFiles(ctx, "ws-1", "u1", 10, token); !errors.Is(err, ErrInvalidPageToken) {
			t.Errorf("token %q: err = %v, want ErrInvalidPageToken", token, err)
		}
	}
	if _, err := f.svc.ListFiles(ctx, "ws-1", "u1", -1, ""); !errors.Is(err, ErrFileInvalid) {
		t.Errorf("negative page size: err = %v", err)
	}
}

func TestDownloadURL(t *testing.T) {
	ctx := context.Background()
	f := newFilesFixture(fakeQuota{limit: -1})
	ready := f.upload(t, "ws-1", 10)
	pending, err := f.svc.CreateUpload(ctx, "ws-1", "u1", NewFile{Name: "p.txt", ContentType: "text/plain", SizeBytes: 3})
	if err != nil {
		t.Fatal(err)
	}

	d, err := f.svc.DownloadURL(ctx, "ws-1", ready.ID)
	if err != nil || d.URL != "https://storage.test/get/"+ready.ObjectKey || !d.ExpiresAt.Equal(filesNow.Add(5*time.Minute)) {
		t.Fatalf("download = %+v, %v", d, err)
	}
	if f.objects.lastName != "a.txt" {
		t.Errorf("download name = %q, want the original name", f.objects.lastName)
	}
	if _, err := f.svc.DownloadURL(ctx, "ws-1", pending.File.ID); !errors.Is(err, ErrFileNotFound) {
		t.Errorf("a pending file must not be downloadable: %v", err)
	}
	if _, err := f.svc.DownloadURL(ctx, "ws-2", ready.ID); !errors.Is(err, ErrFileNotFound) {
		t.Errorf("another workspace's file: err = %v, want ErrFileNotFound", err)
	}
}

func TestDeleteFile(t *testing.T) {
	ctx := context.Background()
	t.Run("removes the object and the record, freeing quota", func(t *testing.T) {
		f := newFilesFixture(fakeQuota{limit: 100})
		file := f.upload(t, "ws-1", 60)
		if err := f.svc.DeleteFile(ctx, "ws-1", file.ID); err != nil {
			t.Fatal(err)
		}
		if len(f.objects.objects) != 0 || f.repo.used("ws-1") != 0 {
			t.Error("object or record left behind")
		}
		if err := f.svc.DeleteFile(ctx, "ws-1", file.ID); !errors.Is(err, ErrFileNotFound) {
			t.Errorf("second delete: %v, want ErrFileNotFound", err)
		}
	})
	t.Run("keeps the record when the object cannot be deleted, so it can be retried", func(t *testing.T) {
		f := newFilesFixture(fakeQuota{limit: 100})
		file := f.upload(t, "ws-1", 60)
		f.objects.failDelete[file.ObjectKey] = true
		if err := f.svc.DeleteFile(ctx, "ws-1", file.ID); err == nil {
			t.Fatal("want the storage error")
		}
		if _, err := f.repo.Get(ctx, "ws-1", file.ID); err != nil {
			t.Errorf("record lost while the object may still exist: %v", err)
		}
	})
	t.Run("another workspace cannot delete it", func(t *testing.T) {
		f := newFilesFixture(fakeQuota{limit: 100})
		file := f.upload(t, "ws-1", 60)
		if err := f.svc.DeleteFile(ctx, "ws-2", file.ID); !errors.Is(err, ErrFileNotFound) {
			t.Fatalf("err = %v, want ErrFileNotFound", err)
		}
		if _, ok := f.objects.objects[file.ObjectKey]; !ok {
			t.Error("a foreign delete removed the object")
		}
	})
}

func TestPurgeAbandoned(t *testing.T) {
	ctx := context.Background()
	f := newFilesFixture(fakeQuota{limit: -1})
	old := filesNow.Add(-2 * time.Hour)

	stale := File{ID: "stale", WorkspaceID: "ws-1", ObjectKey: "workspaces/ws-1/stale", SizeBytes: 5, Status: FilePending, CreatedAt: old}
	fresh := File{ID: "fresh", WorkspaceID: "ws-1", ObjectKey: "workspaces/ws-1/fresh", SizeBytes: 5, Status: FilePending, CreatedAt: filesNow.Add(-time.Minute)}
	erased := File{ID: "erased", WorkspaceID: "ws-9", ObjectKey: "workspaces/ws-9/erased", SizeBytes: 5, Status: FileAbandoned, CreatedAt: filesNow}
	ready := File{ID: "ready", WorkspaceID: "ws-1", ObjectKey: "workspaces/ws-1/ready", SizeBytes: 5, Status: FileReady, CreatedAt: old}
	stuck := File{ID: "stuck", WorkspaceID: "ws-1", ObjectKey: "workspaces/ws-1/stuck", SizeBytes: 5, Status: FilePending, CreatedAt: old}
	for _, file := range []File{stale, fresh, erased, ready, stuck} {
		f.repo.files[k(file.WorkspaceID, file.ID)] = file
		f.objects.objects[file.ObjectKey] = ObjectInfo{Size: 5, ContentType: "text/plain"}
	}
	f.objects.failDelete[stuck.ObjectKey] = true

	n, err := f.svc.PurgeAbandoned(ctx)
	if err == nil || !strings.Contains(err.Error(), "stuck") {
		t.Fatalf("err = %v, want the failure of the stuck object, after the others were handled", err)
	}
	if n != 2 {
		t.Errorf("purged = %d, want 2 (stale and erased)", n)
	}
	for _, gone := range []File{stale, erased} {
		if _, ok := f.repo.files[k(gone.WorkspaceID, gone.ID)]; ok {
			t.Errorf("%s: record left", gone.ID)
		}
		if _, ok := f.objects.objects[gone.ObjectKey]; ok {
			t.Errorf("%s: object left", gone.ID)
		}
	}
	for _, kept := range []File{fresh, ready} {
		if _, ok := f.repo.files[k(kept.WorkspaceID, kept.ID)]; !ok {
			t.Errorf("%s: record removed though it is live", kept.ID)
		}
		if _, ok := f.objects.objects[kept.ObjectKey]; !ok {
			t.Errorf("%s: object removed though it is live", kept.ID)
		}
	}
	// The failed one is now abandoned and keeps its record, so the next run retries it.
	if got := f.repo.files[k("ws-1", "stuck")]; got.Status != FileAbandoned {
		t.Errorf("stuck status = %q, want abandoned (claimed, to be retried)", got.Status)
	}
	f.objects.failDelete[stuck.ObjectKey] = false
	if n, err := f.svc.PurgeAbandoned(ctx); err != nil || n != 1 {
		t.Errorf("retry = %d, %v, want 1, nil", n, err)
	}
}

func TestPurgeAbandoned_LeavesAFileThatCompletedMeanwhile(t *testing.T) {
	f := newFilesFixture(fakeQuota{limit: -1})
	file := File{ID: "x", WorkspaceID: "ws-1", ObjectKey: "workspaces/ws-1/x", SizeBytes: 5, Status: FilePending, CreatedAt: filesNow.Add(-2 * time.Hour)}
	f.repo.files[k("ws-1", "x")] = file
	f.objects.objects[file.ObjectKey] = ObjectInfo{Size: 5, ContentType: "text/plain"}
	race := &racingRepo{fakeFileRepo: f.repo}
	f.svc.repo = race

	n, err := f.svc.PurgeAbandoned(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("purged = %d, %v, want 0, nil", n, err)
	}
	if _, ok := f.objects.objects[file.ObjectKey]; !ok {
		t.Error("the object of a file that was completed during the cleanup was deleted")
	}
}

// racingRepo completes the file between the cleanup's listing and its claim.
type racingRepo struct{ *fakeFileRepo }

func (r *racingRepo) Abandon(ctx context.Context, ws, id string) (bool, error) {
	_, _ = r.MarkReady(ctx, ws, id)
	return r.fakeFileRepo.Abandon(ctx, ws, id)
}

func TestRandomUUID_IsVersion4(t *testing.T) {
	a, b := randomUUID(), randomUUID()
	if a == b || len(a) != 36 || a[14] != '4' || !strings.ContainsRune("89ab", rune(a[19])) {
		t.Errorf("uuids = %s, %s", a, b)
	}
}
