package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/bernardoforcillo/ignition/go-packages/features"
	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"

	saasv1 "github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1"
	"github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1/saasv1connect"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

var fileNow = time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)

// fakeFiles records what the handler hands the use cases and returns canned results.
type fakeFiles struct {
	err   error
	calls []string // "<rpc>:<workspace>:<user or file>"

	upload core.Upload
	page   core.FilePage
}

func (f *fakeFiles) CreateUpload(_ context.Context, ws, user string, in core.NewFile) (core.Upload, error) {
	f.calls = append(f.calls, "create:"+ws+":"+user+":"+in.Name+":"+in.ContentType)
	return f.upload, f.err
}

func (f *fakeFiles) CompleteUpload(_ context.Context, ws, id string) (core.File, error) {
	f.calls = append(f.calls, "complete:"+ws+":"+id)
	return f.upload.File, f.err
}

func (f *fakeFiles) ListFiles(_ context.Context, ws, user string, size int, token string) (core.FilePage, error) {
	f.calls = append(f.calls, "list:"+ws+":"+user+":"+token)
	return f.page, f.err
}

func (f *fakeFiles) DownloadURL(_ context.Context, ws, id string) (core.Download, error) {
	f.calls = append(f.calls, "download:"+ws+":"+id)
	return core.Download{URL: "https://storage.test/get?sig=1", ExpiresAt: fileNow.Add(5 * time.Minute)}, f.err
}

func (f *fakeFiles) DeleteFile(_ context.Context, ws, id string) error {
	f.calls = append(f.calls, "delete:"+ws+":"+id)
	return f.err
}

type filesHarness struct {
	files     *fakeFiles
	workspace *fakeWorkspaces
	client    saasv1connect.FileServiceClient
}

// newFilesHarness serves the SaaS surface with the given file service (nil: storage disabled).
func newFilesHarness(t *testing.T, files fileService) *filesHarness {
	t.Helper()
	h := &filesHarness{workspace: &fakeWorkspaces{}}
	if ff, ok := files.(*fakeFiles); ok {
		h.files = ff
	}
	upstream, _ := url.Parse("http://upstream.invalid")
	app := NewServer(ServerConfig{
		Router:    core.NewRouter([]core.Route{{PathPrefix: "/", Upstream: upstream}}),
		Forwarder: &fakeForwarder{},
		Logger:    discardLogger(),
		SaaS: &SaaS{
			Auth: &fakeAuth{}, Tokens: fakeVerifier{}, Account: &fakeAccount{}, Workspaces: h.workspace,
			Features: &fakeFeatures{}, Files: files, AppURL: "https://app.example.com",
		},
	})
	srv := httptest.NewServer(app)
	t.Cleanup(srv.Close)
	h.client = saasv1connect.NewFileServiceClient(srv.Client(), srv.URL)
	return h
}

func sampleFile() core.File {
	return core.File{
		ID: "f-1", WorkspaceID: "ws-1", Name: "report.pdf", ContentType: "application/pdf", SizeBytes: 1234,
		Status: core.FilePending, ObjectKey: "workspaces/ws-1/f-1", CreatedBy: testUser, CreatedAt: fileNow,
	}
}

// fileRPCs lists every FileService call as a closure, with the permission it needs.
func fileRPCs(c saasv1connect.FileServiceClient, token string) map[string]struct {
	call func() error
	perm string
} {
	type rpc = struct {
		call func() error
		perm string
	}
	ctx := context.Background()
	return map[string]rpc{
		"CreateUpload": {func() error {
			_, err := c.CreateUpload(ctx, bearer(connect.NewRequest(&saasv1.CreateUploadRequest{WorkspaceId: "ws-1", Name: "a.pdf", ContentType: "application/pdf", SizeBytes: 5}), token))
			return err
		}, "file:write"},
		"CompleteUpload": {func() error {
			_, err := c.CompleteUpload(ctx, bearer(connect.NewRequest(&saasv1.CompleteUploadRequest{WorkspaceId: "ws-1", FileId: "f-1"}), token))
			return err
		}, "file:write"},
		"ListFiles": {func() error {
			_, err := c.ListFiles(ctx, bearer(connect.NewRequest(&saasv1.ListFilesRequest{WorkspaceId: "ws-1"}), token))
			return err
		}, "file:read"},
		"GetDownloadUrl": {func() error {
			_, err := c.GetDownloadUrl(ctx, bearer(connect.NewRequest(&saasv1.GetDownloadUrlRequest{WorkspaceId: "ws-1", FileId: "f-1"}), token))
			return err
		}, "file:read"},
		"DeleteFile": {func() error {
			_, err := c.DeleteFile(ctx, bearer(connect.NewRequest(&saasv1.DeleteFileRequest{WorkspaceId: "ws-1", FileId: "f-1"}), token))
			return err
		}, "file:write"},
	}
}

func TestFileService_EveryRPCRequiresAToken(t *testing.T) {
	for name, files := range map[string]fileService{"enabled": &fakeFiles{}, "disabled": nil} {
		h := newFilesHarness(t, files)
		for rpc, r := range fileRPCs(h.client, "") {
			if code := connect.CodeOf(r.call()); code != connect.CodeUnauthenticated {
				t.Errorf("%s/%s without a token: code = %v, want Unauthenticated", name, rpc, code)
			}
		}
		if f, ok := files.(*fakeFiles); ok && len(f.calls) != 0 {
			t.Errorf("an unauthenticated request reached the use cases: %v", f.calls)
		}
	}
}

func TestFileService_DisabledStorageAnswersUnimplemented(t *testing.T) {
	h := newFilesHarness(t, nil)
	for rpc, r := range fileRPCs(h.client, goodToken) {
		if code := connect.CodeOf(r.call()); code != connect.CodeUnimplemented {
			t.Errorf("%s: code = %v, want Unimplemented", rpc, code)
		}
	}
}

func TestFileService_AuthorizesWithTheRightPermission(t *testing.T) {
	for rpc := range fileRPCs(nil, "") {
		h := newFilesHarness(t, &fakeFiles{upload: core.Upload{File: sampleFile()}})
		call := fileRPCs(h.client, goodToken)[rpc]
		if err := call.call(); err != nil {
			t.Fatalf("%s: %v", rpc, err)
		}
		if len(h.workspace.authorized) != 1 || h.workspace.authorized[0] != call.perm {
			t.Errorf("%s authorized %v, want [%s]", rpc, h.workspace.authorized, call.perm)
		}
	}
}

func TestFileService_RefusedCallersNeverReachTheUseCases(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want connect.Code
	}{
		{"a member without the permission", workspace.ErrForbidden, connect.CodePermissionDenied},
		{"someone who is not in the workspace", workspace.ErrNotMember, connect.CodeNotFound},
	}
	for _, tc := range tests {
		for rpc := range fileRPCs(nil, "") {
			t.Run(tc.name+"/"+rpc, func(t *testing.T) {
				h := newFilesHarness(t, &fakeFiles{})
				h.workspace.authzErr = tc.err
				if code := connect.CodeOf(fileRPCs(h.client, goodToken)[rpc].call()); code != tc.want {
					t.Errorf("code = %v, want %v", code, tc.want)
				}
				if len(h.files.calls) != 0 {
					t.Errorf("use cases ran for a refused caller: %v", h.files.calls)
				}
			})
		}
	}
}

func TestFileService_CreateUploadReturnsTheSignedRequest(t *testing.T) {
	ff := &fakeFiles{upload: core.Upload{File: sampleFile(), Request: core.SignedRequest{
		URL: "https://storage.test/put?sig=1", Method: "PUT",
		Headers:   map[string]string{"Content-Type": "application/pdf"},
		ExpiresAt: fileNow.Add(10 * time.Minute),
	}}}
	h := newFilesHarness(t, ff)
	resp, err := h.client.CreateUpload(t.Context(), bearer(connect.NewRequest(&saasv1.CreateUploadRequest{
		WorkspaceId: "ws-1", Name: "report.pdf", ContentType: "application/pdf", SizeBytes: 1234,
	}), goodToken))
	if err != nil {
		t.Fatal(err)
	}
	m := resp.Msg
	if m.GetUploadUrl() != "https://storage.test/put?sig=1" || m.GetUploadMethod() != "PUT" ||
		m.GetUploadHeaders()["Content-Type"] != "application/pdf" || m.GetExpiresAt() != "2026-10-03T09:10:00Z" {
		t.Errorf("response = %v", m)
	}
	if f := m.GetFile(); f.GetId() != "f-1" || f.GetStatus() != "pending" || f.GetCreatedAt() != "2026-10-03T09:00:00Z" || f.GetSizeBytes() != 1234 {
		t.Errorf("file = %v", f)
	}
	if strings.Contains(m.String(), "workspaces/ws-1/f-1") {
		t.Error("the object key must never reach the client")
	}
	if want := "create:ws-1:" + testUser + ":report.pdf:application/pdf"; ff.calls[0] != want {
		t.Errorf("use case got %q, want %q (the caller comes from the token, not the request)", ff.calls[0], want)
	}
}

func TestFileService_ListFilesCarriesPagingAndQuota(t *testing.T) {
	quota := int64(1000)
	ff := &fakeFiles{page: core.FilePage{Files: []core.File{sampleFile()}, NextToken: "next", UsedBytes: 1234, Quota: &quota}}
	h := newFilesHarness(t, ff)
	resp, err := h.client.ListFiles(t.Context(), bearer(connect.NewRequest(&saasv1.ListFilesRequest{WorkspaceId: "ws-1", PageToken: "tok"}), goodToken))
	if err != nil {
		t.Fatal(err)
	}
	m := resp.Msg
	if len(m.GetFiles()) != 1 || m.GetNextPageToken() != "next" || m.GetUsedBytes() != 1234 || m.GetQuotaBytes() != 1000 {
		t.Errorf("response = %v", m)
	}
	if ff.calls[0] != "list:ws-1:"+testUser+":tok" {
		t.Errorf("use case got %q", ff.calls[0])
	}

	ff.page.Quota = nil
	resp, err = h.client.ListFiles(t.Context(), bearer(connect.NewRequest(&saasv1.ListFilesRequest{WorkspaceId: "ws-1"}), goodToken))
	if err != nil || resp.Msg.QuotaBytes != nil {
		t.Errorf("unlimited quota must be absent: %v, %v", resp.Msg, err)
	}
}

func TestFileService_DownloadAndDelete(t *testing.T) {
	ff := &fakeFiles{}
	h := newFilesHarness(t, ff)
	dl, err := h.client.GetDownloadUrl(t.Context(), bearer(connect.NewRequest(&saasv1.GetDownloadUrlRequest{WorkspaceId: "ws-1", FileId: "f-1"}), goodToken))
	if err != nil || dl.Msg.GetUrl() != "https://storage.test/get?sig=1" || dl.Msg.GetExpiresAt() != "2026-10-03T09:05:00Z" {
		t.Fatalf("download = %v, %v", dl, err)
	}
	if _, err := h.client.DeleteFile(t.Context(), bearer(connect.NewRequest(&saasv1.DeleteFileRequest{WorkspaceId: "ws-1", FileId: "f-1"}), goodToken)); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ff.calls, ","); got != "download:ws-1:f-1,delete:ws-1:f-1" {
		t.Errorf("calls = %s", got)
	}
}

func TestFileService_RejectsMissingFields(t *testing.T) {
	h := newFilesHarness(t, &fakeFiles{})
	ctx := t.Context()
	calls := map[string]func() error{
		"create without workspace": func() error {
			_, err := h.client.CreateUpload(ctx, bearer(connect.NewRequest(&saasv1.CreateUploadRequest{Name: "a", ContentType: "a/b", SizeBytes: 1}), goodToken))
			return err
		},
		"create without name": func() error {
			_, err := h.client.CreateUpload(ctx, bearer(connect.NewRequest(&saasv1.CreateUploadRequest{WorkspaceId: "ws-1", ContentType: "a/b", SizeBytes: 1}), goodToken))
			return err
		},
		"create without content type": func() error {
			_, err := h.client.CreateUpload(ctx, bearer(connect.NewRequest(&saasv1.CreateUploadRequest{WorkspaceId: "ws-1", Name: "a", SizeBytes: 1}), goodToken))
			return err
		},
		"complete without file": func() error {
			_, err := h.client.CompleteUpload(ctx, bearer(connect.NewRequest(&saasv1.CompleteUploadRequest{WorkspaceId: "ws-1"}), goodToken))
			return err
		},
		"list without workspace": func() error {
			_, err := h.client.ListFiles(ctx, bearer(connect.NewRequest(&saasv1.ListFilesRequest{}), goodToken))
			return err
		},
		"download without file": func() error {
			_, err := h.client.GetDownloadUrl(ctx, bearer(connect.NewRequest(&saasv1.GetDownloadUrlRequest{WorkspaceId: "ws-1"}), goodToken))
			return err
		},
		"delete without file": func() error {
			_, err := h.client.DeleteFile(ctx, bearer(connect.NewRequest(&saasv1.DeleteFileRequest{WorkspaceId: "ws-1"}), goodToken))
			return err
		},
	}
	for name, call := range calls {
		if code := connect.CodeOf(call()); code != connect.CodeInvalidArgument {
			t.Errorf("%s: code = %v, want InvalidArgument", name, code)
		}
	}
	if len(h.files.calls) != 0 {
		t.Errorf("invalid requests reached the use cases: %v", h.files.calls)
	}
}

func TestFileService_FailuresMapToConnectCodes(t *testing.T) {
	tests := []struct {
		err  error
		want connect.Code
	}{
		{core.ErrFileNotFound, connect.CodeNotFound},
		{core.ErrFileInvalid, connect.CodeInvalidArgument},
		{core.ErrFileTooLarge, connect.CodeInvalidArgument},
		{core.ErrFileTypeNotAllowed, connect.CodeInvalidArgument},
		{core.ErrInvalidPageToken, connect.CodeInvalidArgument},
		{core.ErrQuotaExceeded, connect.CodeResourceExhausted},
		{features.ErrLimitReached, connect.CodeResourceExhausted},
		{features.ErrNotEntitled, connect.CodePermissionDenied},
		{core.ErrUploadMissing, connect.CodeFailedPrecondition},
		{core.ErrUploadMismatch, connect.CodeFailedPrecondition},
		{errBoom, connect.CodeInternal},
		{errors.Join(errBoom, errors.New("https://storage.test/put?X-Amz-Signature=deadbeef")), connect.CodeInternal},
	}
	for _, tc := range tests {
		h := newFilesHarness(t, &fakeFiles{err: tc.err})
		for rpc, r := range fileRPCs(h.client, goodToken) {
			err := r.call()
			if connect.CodeOf(err) != tc.want {
				t.Errorf("%v on %s: code = %v, want %v", tc.err, rpc, connect.CodeOf(err), tc.want)
			}
			if err != nil && (strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "Signature")) {
				t.Errorf("%v on %s leaks internals: %v", tc.err, rpc, err)
			}
		}
	}
}
