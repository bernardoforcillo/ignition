package core

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Errors the file use cases return; the inbound adapter maps each once.
var (
	ErrFileNotFound       = errors.New("file not found")
	ErrFileInvalid        = errors.New("invalid file")
	ErrFileTooLarge       = errors.New("file exceeds the maximum size")
	ErrFileTypeNotAllowed = errors.New("file type not allowed")
	ErrQuotaExceeded      = errors.New("storage quota exceeded")
	// ErrUploadMissing: CompleteUpload found no object, the client has not (finished) uploading.
	ErrUploadMissing = errors.New("uploaded object not found")
	// ErrUploadMismatch: the object that arrived is not what CreateUpload announced.
	ErrUploadMismatch = errors.New("uploaded object does not match the announced file")
	// ErrObjectNotFound is what an ObjectStore reports for a key that holds nothing.
	ErrObjectNotFound = errors.New("object not found")
	// ErrInvalidPageToken: the page token is not one this service issued.
	ErrInvalidPageToken = errors.New("invalid page token")
)

// FileStatus is a file's lifecycle. A file is "pending" from CreateUpload until CompleteUpload
// verified the object, then "ready". "abandoned" is internal: the row waits for the cleanup job to
// remove its object (an upload never completed, or a workspace that was erased); it is invisible to
// every use case here.
type FileStatus string

const (
	FilePending   FileStatus = "pending"
	FileReady     FileStatus = "ready"
	FileAbandoned FileStatus = "abandoned"
)

// File is a workspace's file. ObjectKey is generated here and never shown to a client.
type File struct {
	ID          string
	WorkspaceID string
	// Name is the original name, for display and as the download name only.
	Name        string
	ContentType string
	SizeBytes   int64
	Status      FileStatus
	ObjectKey   string
	CreatedBy   string
	CreatedAt   time.Time
}

// FileCursor is the keyset position of a page: files are listed newest first, by (CreatedAt, ID).
type FileCursor struct {
	CreatedAt time.Time
	ID        string
}

// FileRepository is where file records live (the saas adapter implements it over Postgres). Every
// method is scoped by workspace id, so a record of another workspace is simply not found.
type FileRepository interface {
	// Reserve inserts a pending file unless the workspace's pending and ready bytes plus the new
	// file's would exceed quota (negative quota: unlimited). The check and the insert are atomic
	// across replicas. It returns ErrQuotaExceeded on refusal.
	Reserve(ctx context.Context, f File, quota int64) error
	// Get returns a pending or ready file, or ErrFileNotFound.
	Get(ctx context.Context, workspaceID, id string) (File, error)
	// MarkReady turns a pending file ready (a ready one stays ready) and returns it.
	MarkReady(ctx context.Context, workspaceID, id string) (File, error)
	// ListReady returns up to limit ready files strictly after the cursor (nil: from the newest).
	ListReady(ctx context.Context, workspaceID string, after *FileCursor, limit int) ([]File, error)
	// UsedBytes is what pending and ready files occupy.
	UsedBytes(ctx context.Context, workspaceID string) (int64, error)
	// Delete removes the record; a missing one is ErrFileNotFound.
	Delete(ctx context.Context, workspaceID, id string) error
	// Abandon marks a pending file abandoned and reports whether this call did (a file completed
	// meanwhile is left alone).
	Abandon(ctx context.Context, workspaceID, id string) (bool, error)
	// ListAbandoned returns files the cleanup should remove: abandoned ones, and pending ones
	// created before the cutoff.
	ListAbandoned(ctx context.Context, pendingBefore time.Time, limit int) ([]File, error)
	// DeleteAbandoned removes the record of an abandoned file, once its object is gone.
	DeleteAbandoned(ctx context.Context, workspaceID, id string) error
}

// SignedRequest is a request the client may perform without credentials.
type SignedRequest struct {
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

// ObjectInfo is what the object store holds for a key.
type ObjectInfo struct {
	Size        int64
	ContentType string
}

// ObjectStore is the blob storage the files live in, behind signed URLs: the bytes never pass
// through this service. Delete of a missing object succeeds; Stat reports ErrObjectNotFound.
type ObjectStore interface {
	SignUpload(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (SignedRequest, error)
	SignDownload(ctx context.Context, key, filename string, ttl time.Duration) (SignedRequest, error)
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	Delete(ctx context.Context, key string) error
}

// StorageQuota resolves how many bytes a workspace may keep (negative: unlimited). It fails closed:
// a workspace whose plan has no storage, or a store outage, is an error, not "unlimited".
type StorageQuota interface {
	Limit(ctx context.Context, workspaceID, userID string) (int64, error)
}

// FileLimits are the product's rules for uploads.
type FileLimits struct {
	// MaxFileBytes caps one file.
	MaxFileBytes int64
	// AllowedTypes is the exact media types (lower-case, no parameters) a file may have.
	AllowedTypes []string
	// UploadTTL and DownloadTTL are how long the signed URLs live.
	UploadTTL, DownloadTTL time.Duration
	// PendingTTL is how long an upload may stay unconfirmed before the cleanup removes it. It must
	// be comfortably longer than UploadTTL.
	PendingTTL time.Duration
}

const (
	maxNameRunes = 255
	// DefaultFilePageSize and MaxFilePageSize bound ListFiles.
	DefaultFilePageSize = 50
	MaxFilePageSize     = 200
	// purgeBatch is how many abandoned files one cleanup call handles.
	purgeBatch = 100
)

// Files are the workspace file use cases. Authorization is the caller's: it has already checked
// that the user may read or write files of the workspace.
type Files struct {
	repo    FileRepository
	objects ObjectStore
	quota   StorageQuota
	limits  FileLimits
	now     func() time.Time
	newID   func() string
}

// NewFiles wires the use cases. A nil now or newID means the real clock and random UUIDs.
func NewFiles(repo FileRepository, objects ObjectStore, quota StorageQuota, limits FileLimits, now func() time.Time, newID func() string) *Files {
	if now == nil {
		now = time.Now
	}
	if newID == nil {
		newID = randomUUID
	}
	return &Files{repo: repo, objects: objects, quota: quota, limits: limits, now: now, newID: newID}
}

// NewFile is what a client announces.
type NewFile struct {
	Name        string
	ContentType string
	SizeBytes   int64
}

// Upload is a registered pending file and the request that uploads its bytes.
type Upload struct {
	File    File
	Request SignedRequest
}

// CreateUpload validates the announced file, reserves its bytes against the workspace quota and
// returns the signed request that uploads them. The object key is generated here from the workspace
// and a fresh id; nothing the client sent reaches it.
func (s *Files) CreateUpload(ctx context.Context, workspaceID, userID string, in NewFile) (Upload, error) {
	if workspaceID == "" || strings.ContainsAny(workspaceID, "/\\") {
		return Upload{}, fmt.Errorf("%w: workspace", ErrFileInvalid)
	}
	name, err := cleanName(in.Name)
	if err != nil {
		return Upload{}, err
	}
	ct, err := mediaType(in.ContentType)
	if err != nil {
		return Upload{}, err
	}
	if !slices.Contains(s.limits.AllowedTypes, ct) {
		return Upload{}, ErrFileTypeNotAllowed
	}
	switch {
	case in.SizeBytes <= 0:
		return Upload{}, fmt.Errorf("%w: size", ErrFileInvalid)
	case in.SizeBytes > s.limits.MaxFileBytes:
		return Upload{}, ErrFileTooLarge
	}
	quota, err := s.quota.Limit(ctx, workspaceID, userID)
	if err != nil {
		return Upload{}, err
	}

	id := s.newID()
	f := File{
		ID: id, WorkspaceID: workspaceID, Name: name, ContentType: ct, SizeBytes: in.SizeBytes,
		Status: FilePending, ObjectKey: "workspaces/" + workspaceID + "/" + id,
		CreatedBy: userID, CreatedAt: s.now().UTC().Truncate(time.Microsecond),
	}
	if err := s.repo.Reserve(ctx, f, quota); err != nil {
		return Upload{}, err
	}
	req, err := s.objects.SignUpload(ctx, f.ObjectKey, ct, f.SizeBytes, s.limits.UploadTTL)
	if err != nil {
		// Give the reservation back; the cleanup job is the backstop if this fails too.
		if derr := s.repo.Delete(ctx, workspaceID, id); derr != nil {
			err = errors.Join(err, derr)
		}
		return Upload{}, err
	}
	return Upload{File: f, Request: req}, nil
}

// CompleteUpload confirms an upload: the object must exist with the announced size and type. It is
// idempotent for a file that is already ready. A mismatching object is deleted along with the
// record, so a wrong upload cannot occupy quota.
func (s *Files) CompleteUpload(ctx context.Context, workspaceID, id string) (File, error) {
	f, err := s.repo.Get(ctx, workspaceID, id)
	if err != nil {
		return File{}, err
	}
	if f.Status == FileReady {
		return f, nil
	}
	obj, err := s.objects.Stat(ctx, f.ObjectKey)
	switch {
	case errors.Is(err, ErrObjectNotFound):
		return File{}, ErrUploadMissing
	case err != nil:
		return File{}, err
	}
	if got, err := mediaType(obj.ContentType); obj.Size != f.SizeBytes || err != nil || got != f.ContentType {
		if err := s.objects.Delete(ctx, f.ObjectKey); err != nil {
			return File{}, err
		}
		if err := s.repo.Delete(ctx, workspaceID, id); err != nil && !errors.Is(err, ErrFileNotFound) {
			return File{}, err
		}
		return File{}, ErrUploadMismatch
	}
	return s.repo.MarkReady(ctx, workspaceID, id)
}

// FilePage is one page of ready files, newest first, with the workspace's storage figures.
type FilePage struct {
	Files []File
	// NextToken is empty on the last page.
	NextToken string
	// UsedBytes counts pending and ready files; Quota is the plan's allowance, nil when unlimited.
	UsedBytes int64
	Quota     *int64
}

// ListFiles returns a page of the workspace's ready files. pageSize 0 means the default; token is
// the NextToken of the previous page.
func (s *Files) ListFiles(ctx context.Context, workspaceID, userID string, pageSize int, token string) (FilePage, error) {
	switch {
	case pageSize < 0:
		return FilePage{}, fmt.Errorf("%w: page size", ErrFileInvalid)
	case pageSize == 0:
		pageSize = DefaultFilePageSize
	case pageSize > MaxFilePageSize:
		pageSize = MaxFilePageSize
	}
	var after *FileCursor
	if token != "" {
		c, err := decodeCursor(token)
		if err != nil {
			return FilePage{}, err
		}
		after = &c
	}
	// One extra row tells whether there is a next page without a second query.
	files, err := s.repo.ListReady(ctx, workspaceID, after, pageSize+1)
	if err != nil {
		return FilePage{}, err
	}
	page := FilePage{}
	if len(files) > pageSize {
		files = files[:pageSize]
		page.NextToken = encodeCursor(FileCursor{CreatedAt: files[pageSize-1].CreatedAt, ID: files[pageSize-1].ID})
	}
	page.Files = files
	if page.UsedBytes, err = s.repo.UsedBytes(ctx, workspaceID); err != nil {
		return FilePage{}, err
	}
	limit, err := s.quota.Limit(ctx, workspaceID, userID)
	if err != nil {
		return FilePage{}, err
	}
	if limit >= 0 {
		page.Quota = &limit
	}
	return page, nil
}

// Download is a short-lived signed GET for one file.
type Download struct {
	URL       string
	ExpiresAt time.Time
}

// DownloadURL signs a download of a ready file. The response is an attachment under the file's
// original name, so an uploaded document is saved, never rendered by the browser.
func (s *Files) DownloadURL(ctx context.Context, workspaceID, id string) (Download, error) {
	f, err := s.repo.Get(ctx, workspaceID, id)
	if err != nil {
		return Download{}, err
	}
	if f.Status != FileReady {
		return Download{}, ErrFileNotFound
	}
	req, err := s.objects.SignDownload(ctx, f.ObjectKey, f.Name, s.limits.DownloadTTL)
	if err != nil {
		return Download{}, err
	}
	return Download{URL: req.URL, ExpiresAt: req.ExpiresAt}, nil
}

// DeleteFile removes the object, then the record: a failure in between leaves a record that can be
// deleted again, never an object nobody knows about.
func (s *Files) DeleteFile(ctx context.Context, workspaceID, id string) error {
	f, err := s.repo.Get(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	if err := s.objects.Delete(ctx, f.ObjectKey); err != nil {
		return err
	}
	return s.repo.Delete(ctx, workspaceID, id)
}

// PurgeAbandoned removes what will never be used: uploads never confirmed within PendingTTL and
// files of erased workspaces. For each it first claims the record (so a CompleteUpload racing with
// the cleanup either wins before, or finds nothing after), deletes the object, then the record. It
// returns how many it removed; a failure on one file does not stop the others.
func (s *Files) PurgeAbandoned(ctx context.Context) (int, error) {
	files, err := s.repo.ListAbandoned(ctx, s.now().Add(-s.limits.PendingTTL), purgeBatch)
	if err != nil {
		return 0, err
	}
	var errs []error
	purged := 0
	for _, f := range files {
		if f.Status == FilePending {
			if claimed, err := s.repo.Abandon(ctx, f.WorkspaceID, f.ID); err != nil {
				errs = append(errs, err)
				continue
			} else if !claimed {
				continue // completed or deleted meanwhile
			}
		}
		if err := s.objects.Delete(ctx, f.ObjectKey); err != nil {
			errs = append(errs, fmt.Errorf("deleting object of file %s: %w", f.ID, err))
			continue
		}
		if err := s.repo.DeleteAbandoned(ctx, f.WorkspaceID, f.ID); err != nil {
			errs = append(errs, err)
			continue
		}
		purged++
	}
	return purged, errors.Join(errs...)
}

// cleanName trims a display name and rejects what cannot be shown or put in a header.
func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameRunes || !utf8.ValidString(name) {
		return "", fmt.Errorf("%w: name", ErrFileInvalid)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("%w: name", ErrFileInvalid)
		}
	}
	return name, nil
}

// mediaType reduces a Content-Type to its lower-case "type/subtype".
func mediaType(contentType string) (string, error) {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.Contains(mt, "/") {
		return "", fmt.Errorf("%w: content type", ErrFileInvalid)
	}
	return mt, nil
}

// The page token is the position of the last file shown, base64url of "<unix micros>.<id>". It is
// not secret and not signed: it only says where to continue, and every list is workspace-scoped.
func encodeCursor(c FileCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.CreatedAt.UnixMicro(), 10) + "." + c.ID))
}

func decodeCursor(token string) (FileCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return FileCursor{}, ErrInvalidPageToken
	}
	micros, id, ok := strings.Cut(string(raw), ".")
	n, perr := strconv.ParseInt(micros, 10, 64)
	if !ok || perr != nil || id == "" {
		return FileCursor{}, ErrInvalidPageToken
	}
	return FileCursor{CreatedAt: time.UnixMicro(n).UTC(), ID: id}, nil
}

// randomUUID is a version 4 UUID from crypto/rand.
func randomUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("core: no randomness: " + err.Error()) // crypto/rand does not fail on supported platforms
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
