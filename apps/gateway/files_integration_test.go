package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/bernardoforcillo/ignition/go-packages/database"
	"github.com/bernardoforcillo/ignition/go-packages/identity/auth"
	jobslib "github.com/bernardoforcillo/ignition/go-packages/jobs"
	saasv1 "github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1"
	"github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1/saasv1connect"
	"github.com/bernardoforcillo/ignition/go-packages/storage/storagetest"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/adapter/httpapi"
	jobsadapter "github.com/bernardoforcillo/ignition/apps/gateway/internal/adapter/jobs"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/adapter/objectstore"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/adapter/proxy"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/adapter/saas"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/config"
	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

// tokenVerifier maps fixed test tokens to account ids; everything else (workspaces, roles, the
// plan, the quota, the SQL) is the real thing.
type tokenVerifier map[string]string

func (v tokenVerifier) VerifyAccessToken(raw string) (auth.Claims, error) {
	id, ok := v[raw]
	if !ok {
		return auth.Claims{}, auth.ErrTokenInvalid
	}
	return auth.Claims{Subject: id}, nil
}

// fileStack is the gateway's SaaS surface over a private schema and the in-memory storage server.
type fileStack struct {
	svc     *saas.Services
	storage *storagetest.Server
	url     string
	db      *database.DB
	cfg     config.SaaS
}

func newFileStack(t *testing.T, db *database.DB, srv *storagetest.Server, tokens tokenVerifier, withFiles bool) *fileStack {
	t.Helper()
	cfg := config.SaaS{
		AuthSecret: []byte(strings.Repeat("s", 32)), AccessTTL: time.Minute, RefreshTTL: time.Hour,
		AppURL: "https://app.example.com", CompanyName: "Acme", MailFrom: "Acme <hi@example.com>",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var opts []saas.Option
	if withFiles {
		opts = append(opts, saas.WithFiles(objectstore.Wrap(srv.S3(t)), core.FileLimits{
			MaxFileBytes: 80 << 20, AllowedTypes: []string{"text/plain", "application/pdf"},
			UploadTTL: time.Minute, DownloadTTL: time.Minute, PendingTTL: time.Hour,
		}))
	}
	svc, err := saas.Build(t.Context(), cfg, db, logger, opts...)
	if err != nil {
		t.Fatal(err)
	}
	api := newSaaSAPI(svc, cfg.AppURL, nil)
	api.Tokens = tokens
	upstream, _ := url.Parse("http://upstream.invalid")
	app := httpapi.NewServer(httpapi.ServerConfig{
		Router:    core.NewRouter([]core.Route{{PathPrefix: "/", Upstream: upstream}}),
		Forwarder: proxy.New(), Logger: logger, SaaS: api,
	})
	ts := httptest.NewServer(app)
	t.Cleanup(ts.Close)
	return &fileStack{svc: svc, storage: srv, url: ts.URL, db: db, cfg: cfg}
}

// as returns a FileService client that authenticates with the given test token.
func (s *fileStack) as(token string) saasv1connect.FileServiceClient {
	bearer := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("Authorization", "Bearer "+token)
			return next(ctx, req)
		}
	})
	return saasv1connect.NewFileServiceClient(http.DefaultClient, s.url, connect.WithInterceptors(bearer))
}

// putTo performs the upload the way a browser would: straight to the storage server, with the
// headers the gateway returned.
func putTo(t *testing.T, up *saasv1.CreateUploadResponse, body []byte) int {
	t.Helper()
	req, err := http.NewRequest(up.GetUploadMethod(), up.GetUploadUrl(), bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range up.GetUploadHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func createUpload(c saasv1connect.FileServiceClient, ws, name, contentType string, size int64) (*saasv1.CreateUploadResponse, error) {
	resp, err := c.CreateUpload(context.Background(), connect.NewRequest(&saasv1.CreateUploadRequest{
		WorkspaceId: ws, Name: name, ContentType: contentType, SizeBytes: size,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func wantCode(t *testing.T, what string, err error, want connect.Code) {
	t.Helper()
	if got := connect.CodeOf(err); got != want || err == nil {
		t.Errorf("%s: code = %v (%v), want %v", what, got, err, want)
	}
}

// TestFileService_FullStackAgainstPostgres drives the real RPC surface: the auth interceptor, the
// workspace RBAC, the plan's quota, the SQL and the signing adapter, with the in-memory storage
// server standing in for the bucket. The bytes only ever go to that server.
func TestFileService_FullStackAgainstPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("needs external service")
	}
	db := scratchDB(t)
	ctx := t.Context()
	srv := storagetest.New(t)
	tokens := tokenVerifier{}
	stack := newFileStack(t, db, srv, tokens, true)

	// ada owns wsA; bob is a plain member of it (no file permissions by default); carl owns wsC.
	users := map[string]string{}
	for _, name := range []string{"ada", "bob", "carl"} {
		rows, err := db.Query(ctx, `INSERT INTO users (id, email, password_hash, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, 'x', now(), now()) RETURNING id::text`, name+"@example.com")
		if err != nil {
			t.Fatal(err)
		}
		if rows.Next() {
			var id string
			_ = rows.Scan(&id)
			users[name] = id
			tokens["tok-"+name] = id
		}
		_ = rows.Close()
	}
	wsOf := func(owner, name string) string {
		ws, err := stack.svc.Workspaces.Create(ctx, users[owner], name, "")
		if err != nil {
			t.Fatal(err)
		}
		return ws.ID
	}
	wsA, wsC := wsOf("ada", "Ada Co"), wsOf("carl", "Carl Co")
	exec(t, db.DB, `INSERT INTO organization_members (container_id, user_id, role_key, joined_at) VALUES ($1::uuid, $2::uuid, 'member', now())`, wsA, users["bob"])
	ada, bob, carl := stack.as("tok-ada"), stack.as("tok-bob"), stack.as("tok-carl")

	t.Run("upload, list, download, delete", func(t *testing.T) {
		body := []byte("hello from the full stack")
		up, err := createUpload(ada, wsA, "hello.txt", "text/plain; charset=utf-8", int64(len(body)))
		if err != nil {
			t.Fatal(err)
		}
		if up.GetFile().GetStatus() != "pending" || !strings.HasPrefix(up.GetUploadUrl(), srv.URL+"/"+storagetest.Bucket+"/workspaces/"+wsA+"/") {
			t.Fatalf("upload = %v", up)
		}
		if up.GetUploadHeaders()["Content-Type"] != "text/plain" {
			t.Errorf("headers = %v, want the normalized content type", up.GetUploadHeaders())
		}

		// Not uploaded yet: completing is refused, and the file is not listed.
		_, err = ada.CompleteUpload(ctx, connect.NewRequest(&saasv1.CompleteUploadRequest{WorkspaceId: wsA, FileId: up.GetFile().GetId()}))
		wantCode(t, "complete before upload", err, connect.CodeFailedPrecondition)
		if list, _ := ada.ListFiles(ctx, connect.NewRequest(&saasv1.ListFilesRequest{WorkspaceId: wsA})); len(list.Msg.GetFiles()) != 0 {
			t.Error("a pending file was listed")
		}

		// The storage edge refuses an upload of another size or type, so nothing wrong lands.
		if code := putTo(t, &saasv1.CreateUploadResponse{UploadUrl: up.GetUploadUrl(), UploadMethod: "PUT", UploadHeaders: up.GetUploadHeaders()}, append(body, '!')); code < 400 {
			t.Errorf("a longer body than announced was accepted (%d)", code)
		}
		if code := putTo(t, &saasv1.CreateUploadResponse{UploadUrl: up.GetUploadUrl(), UploadMethod: "PUT", UploadHeaders: map[string]string{"Content-Type": "text/html"}}, body); code < 400 {
			t.Errorf("another content type was accepted (%d)", code)
		}
		if code := putTo(t, up, body); code != http.StatusOK {
			t.Fatalf("the real upload got %d", code)
		}

		done, err := ada.CompleteUpload(ctx, connect.NewRequest(&saasv1.CompleteUploadRequest{WorkspaceId: wsA, FileId: up.GetFile().GetId()}))
		if err != nil || done.Msg.GetFile().GetStatus() != "ready" {
			t.Fatalf("complete = %v, %v", done, err)
		}
		list, err := ada.ListFiles(ctx, connect.NewRequest(&saasv1.ListFilesRequest{WorkspaceId: wsA}))
		if err != nil || len(list.Msg.GetFiles()) != 1 || list.Msg.GetUsedBytes() != int64(len(body)) || list.Msg.GetQuotaBytes() != 100<<20 {
			t.Fatalf("list = %v, %v", list, err)
		}

		dl, err := ada.GetDownloadUrl(ctx, connect.NewRequest(&saasv1.GetDownloadUrlRequest{WorkspaceId: wsA, FileId: up.GetFile().GetId()}))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.Get(dl.Msg.GetUrl())
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if !bytes.Equal(got, body) || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment;") {
			t.Errorf("download = %q, disposition %q", got, resp.Header.Get("Content-Disposition"))
		}

		if _, err := ada.DeleteFile(ctx, connect.NewRequest(&saasv1.DeleteFileRequest{WorkspaceId: wsA, FileId: up.GetFile().GetId()})); err != nil {
			t.Fatal(err)
		}
		if len(srv.Keys()) != 0 {
			t.Errorf("objects left in the bucket: %v", srv.Keys())
		}
		_, err = ada.GetDownloadUrl(ctx, connect.NewRequest(&saasv1.GetDownloadUrlRequest{WorkspaceId: wsA, FileId: up.GetFile().GetId()}))
		wantCode(t, "download after delete", err, connect.CodeNotFound)
	})

	t.Run("type and size rules", func(t *testing.T) {
		_, err := createUpload(ada, wsA, "page.html", "text/html", 10)
		wantCode(t, "html", err, connect.CodeInvalidArgument)
		_, err = createUpload(ada, wsA, "huge.txt", "text/plain", 81<<20)
		wantCode(t, "over the max size", err, connect.CodeInvalidArgument)
		_, err = createUpload(ada, wsA, "empty.txt", "text/plain", 0)
		wantCode(t, "empty", err, connect.CodeInvalidArgument)
		if n := scalar(t, db.DB, `SELECT count(*) FROM files WHERE status = 'pending'`); n != 0 {
			t.Errorf("%d rows reserved by refused uploads", n)
		}
	})

	t.Run("roles and tenants", func(t *testing.T) {
		// A member holds no file permissions by default.
		_, err := createUpload(bob, wsA, "a.txt", "text/plain", 5)
		wantCode(t, "member uploading", err, connect.CodePermissionDenied)
		_, err = bob.ListFiles(ctx, connect.NewRequest(&saasv1.ListFilesRequest{WorkspaceId: wsA}))
		wantCode(t, "member listing", err, connect.CodePermissionDenied)

		// carl's file in his own workspace.
		up, err := createUpload(carl, wsC, "secret.txt", "text/plain", 6)
		if err != nil {
			t.Fatal(err)
		}
		if code := putTo(t, up, []byte("secret")); code != http.StatusOK {
			t.Fatalf("upload got %d", code)
		}
		if _, err := carl.CompleteUpload(ctx, connect.NewRequest(&saasv1.CompleteUploadRequest{WorkspaceId: wsC, FileId: up.GetFile().GetId()})); err != nil {
			t.Fatal(err)
		}
		id := up.GetFile().GetId()

		// ada is not in wsC: she learns nothing, whichever RPC she tries.
		_, err = createUpload(ada, wsC, "x.txt", "text/plain", 1)
		wantCode(t, "create in a foreign workspace", err, connect.CodeNotFound)
		_, err = ada.ListFiles(ctx, connect.NewRequest(&saasv1.ListFilesRequest{WorkspaceId: wsC}))
		wantCode(t, "list a foreign workspace", err, connect.CodeNotFound)
		_, err = ada.GetDownloadUrl(ctx, connect.NewRequest(&saasv1.GetDownloadUrlRequest{WorkspaceId: wsC, FileId: id}))
		wantCode(t, "download a foreign file", err, connect.CodeNotFound)
		_, err = ada.DeleteFile(ctx, connect.NewRequest(&saasv1.DeleteFileRequest{WorkspaceId: wsC, FileId: id}))
		wantCode(t, "delete a foreign file", err, connect.CodeNotFound)

		// A real file id presented under her own workspace id does not leak either.
		_, err = ada.GetDownloadUrl(ctx, connect.NewRequest(&saasv1.GetDownloadUrlRequest{WorkspaceId: wsA, FileId: id}))
		wantCode(t, "foreign id under own workspace", err, connect.CodeNotFound)
		_, err = ada.DeleteFile(ctx, connect.NewRequest(&saasv1.DeleteFileRequest{WorkspaceId: wsA, FileId: id}))
		wantCode(t, "delete foreign id under own workspace", err, connect.CodeNotFound)
		_, err = ada.CompleteUpload(ctx, connect.NewRequest(&saasv1.CompleteUploadRequest{WorkspaceId: wsA, FileId: id}))
		wantCode(t, "complete foreign id under own workspace", err, connect.CodeNotFound)

		if list, err := carl.ListFiles(ctx, connect.NewRequest(&saasv1.ListFilesRequest{WorkspaceId: wsC})); err != nil || len(list.Msg.GetFiles()) != 1 {
			t.Errorf("carl's file was affected: %v, %v", list, err)
		}
		if _, err := ada.ListFiles(ctx, connect.NewRequest(&saasv1.ListFilesRequest{WorkspaceId: wsA})); err != nil {
			t.Errorf("ada lists her own workspace: %v", err)
		}
	})

	t.Run("a bad token is refused before anything else", func(t *testing.T) {
		_, err := createUpload(stack.as("nope"), wsA, "a.txt", "text/plain", 5)
		wantCode(t, "bad token", err, connect.CodeUnauthenticated)
	})

	t.Run("quota follows the plan and frees on delete", func(t *testing.T) {
		const big = 70 << 20 // free plan: 100 MiB
		first, err := createUpload(ada, wsA, "big1.txt", "text/plain", big)
		if err != nil {
			t.Fatal(err)
		}
		_, err = createUpload(ada, wsA, "big2.txt", "text/plain", big)
		wantCode(t, "second 70 MiB on a 100 MiB plan", err, connect.CodeResourceExhausted)
		// Another workspace has its own allowance.
		if _, err := createUpload(carl, wsC, "big.txt", "text/plain", big); err != nil {
			t.Errorf("carl's quota was charged for ada's reservation: %v", err)
		}
		if _, err := ada.DeleteFile(ctx, connect.NewRequest(&saasv1.DeleteFileRequest{WorkspaceId: wsA, FileId: first.GetFile().GetId()})); err != nil {
			t.Fatal(err)
		}
		if _, err := createUpload(ada, wsA, "big2.txt", "text/plain", big); err != nil {
			t.Errorf("deleting a file must free its bytes: %v", err)
		}
	})

	t.Run("abandoned uploads are cleaned up by the job", func(t *testing.T) {
		exec(t, db.DB, `DELETE FROM files`)
		for id, ago := range map[string]time.Duration{"stale": 2 * time.Hour, "fresh": time.Minute} {
			exec(t, db.DB, `INSERT INTO files (id, workspace_id, name, content_type, size_bytes, status, object_key, created_by, created_at)
				VALUES ($1, $2, $1, 'text/plain', 5, 'pending', $3, 'u', $4)`, id, wsA, "workspaces/"+wsA+"/"+id, time.Now().Add(-ago))
			srv.Put("workspaces/"+wsA+"/"+id, []byte("hello"), "text/plain")
		}
		runner, err := jobsadapter.New(
			jobsadapter.Deps{DB: stack.svc.DB(), Owners: stack.svc, Mail: stack.svc.Mail, AppURL: stack.cfg.AppURL, Files: stack.svc.Files,
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
			jobsadapter.WithEvery(time.Second), jobsadapter.WithSchedulerOptions(jobslib.WithJitter(0)),
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := runner.Start(ctx); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for scalar(t, db.DB, `SELECT count(*) FROM files WHERE id = 'stale'`) != 0 {
			if time.Now().After(deadline) {
				t.Fatal("files.cleanup did not remove the stale upload")
			}
			time.Sleep(50 * time.Millisecond)
		}
		if err := runner.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		if _, _, ok := srv.Get("workspaces/" + wsA + "/stale"); ok {
			t.Error("the stale upload's object is still in the bucket")
		}
		if _, _, ok := srv.Get("workspaces/" + wsA + "/fresh"); !ok || scalar(t, db.DB, `SELECT count(*) FROM files WHERE id = 'fresh'`) != 1 {
			t.Error("an upload still within its window was cleaned up")
		}
	})
}

// TestFileService_DisabledStorageIsUnimplemented: with no object store configured FileService is
// mounted and authenticated, and answers Unimplemented; the rest of the SaaS surface is unaffected.
func TestFileService_DisabledStorageIsUnimplemented(t *testing.T) {
	if testing.Short() {
		t.Skip("needs external service")
	}
	db := scratchDB(t)
	stack := newFileStack(t, db, nil, tokenVerifier{"tok": "11111111-1111-1111-1111-111111111111"}, false)
	if stack.svc.Files != nil {
		t.Fatal("Files must be nil without an object store")
	}
	c := stack.as("tok")
	ctx := t.Context()
	_, err := createUpload(c, "ws", "a.txt", "text/plain", 1)
	wantCode(t, "CreateUpload", err, connect.CodeUnimplemented)
	_, err = c.ListFiles(ctx, connect.NewRequest(&saasv1.ListFilesRequest{WorkspaceId: "ws"}))
	wantCode(t, "ListFiles", err, connect.CodeUnimplemented)
	_, err = c.GetDownloadUrl(ctx, connect.NewRequest(&saasv1.GetDownloadUrlRequest{WorkspaceId: "ws", FileId: "f"}))
	wantCode(t, "GetDownloadUrl", err, connect.CodeUnimplemented)
	_, err = c.CompleteUpload(ctx, connect.NewRequest(&saasv1.CompleteUploadRequest{WorkspaceId: "ws", FileId: "f"}))
	wantCode(t, "CompleteUpload", err, connect.CodeUnimplemented)
	_, err = c.DeleteFile(ctx, connect.NewRequest(&saasv1.DeleteFileRequest{WorkspaceId: "ws", FileId: "f"}))
	wantCode(t, "DeleteFile", err, connect.CodeUnimplemented)
	_, err = stack.as("bad").ListFiles(ctx, connect.NewRequest(&saasv1.ListFilesRequest{WorkspaceId: "ws"}))
	wantCode(t, "unauthenticated", err, connect.CodeUnauthenticated)
}
