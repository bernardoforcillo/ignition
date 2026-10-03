package httpapi

import (
	"context"
	"time"

	"connectrpc.com/connect"

	"github.com/bernardoforcillo/ignition/go-packages/identity/permissions"

	saasv1 "github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1"

	"github.com/bernardoforcillo/ignition/apps/gateway/internal/core"
)

// fileService is the file use cases the handler needs; *core.Files satisfies it.
type fileService interface {
	CreateUpload(ctx context.Context, workspaceID, userID string, in core.NewFile) (core.Upload, error)
	CompleteUpload(ctx context.Context, workspaceID, id string) (core.File, error)
	ListFiles(ctx context.Context, workspaceID, userID string, pageSize int, token string) (core.FilePage, error)
	DownloadURL(ctx context.Context, workspaceID, id string) (core.Download, error)
	DeleteFile(ctx context.Context, workspaceID, id string) error
}

// fileHandler implements saasv1connect.FileServiceHandler. Every RPC authenticates through the
// interceptor, then authorizes against the workspace (file:read to look, file:write to change; a
// non-member learns nothing, not even that the workspace exists) and delegates to the use cases,
// which scope every record by the authorized workspace id.
type fileHandler struct {
	files fileService
	authz workspaceAuthorizer
}

// authorize is the shared prelude: the caller, a workspace id, and the permission on its files.
func (h *fileHandler) authorize(ctx context.Context, workspaceID string, write bool) (string, error) {
	user, err := subjectOrErr(ctx)
	if err != nil {
		return "", err
	}
	if workspaceID == "" {
		return "", requiredField("workspace_id")
	}
	action := permissions.ActionRead
	if write {
		action = permissions.ActionWrite
	}
	if err := h.authz.Authorize(ctx, user, workspaceID, permissions.ResourceFile, action); err != nil {
		return "", toConnectError(ctx, err)
	}
	return user, nil
}

func (h *fileHandler) CreateUpload(ctx context.Context, req *connect.Request[saasv1.CreateUploadRequest]) (*connect.Response[saasv1.CreateUploadResponse], error) {
	m := req.Msg
	switch {
	case m.GetName() == "":
		return nil, requiredField("name")
	case m.GetContentType() == "":
		return nil, requiredField("content_type")
	}
	user, err := h.authorize(ctx, m.GetWorkspaceId(), true)
	if err != nil {
		return nil, err
	}
	up, err := h.files.CreateUpload(ctx, m.GetWorkspaceId(), user, core.NewFile{
		Name: m.GetName(), ContentType: m.GetContentType(), SizeBytes: m.GetSizeBytes(),
	})
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.CreateUploadResponse{
		File:          fileMessage(up.File),
		UploadUrl:     up.Request.URL,
		UploadMethod:  up.Request.Method,
		UploadHeaders: up.Request.Headers,
		ExpiresAt:     up.Request.ExpiresAt.UTC().Format(time.RFC3339),
	}), nil
}

func (h *fileHandler) CompleteUpload(ctx context.Context, req *connect.Request[saasv1.CompleteUploadRequest]) (*connect.Response[saasv1.CompleteUploadResponse], error) {
	if req.Msg.GetFileId() == "" {
		return nil, requiredField("file_id")
	}
	if _, err := h.authorize(ctx, req.Msg.GetWorkspaceId(), true); err != nil {
		return nil, err
	}
	f, err := h.files.CompleteUpload(ctx, req.Msg.GetWorkspaceId(), req.Msg.GetFileId())
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.CompleteUploadResponse{File: fileMessage(f)}), nil
}

func (h *fileHandler) ListFiles(ctx context.Context, req *connect.Request[saasv1.ListFilesRequest]) (*connect.Response[saasv1.ListFilesResponse], error) {
	user, err := h.authorize(ctx, req.Msg.GetWorkspaceId(), false)
	if err != nil {
		return nil, err
	}
	page, err := h.files.ListFiles(ctx, req.Msg.GetWorkspaceId(), user, int(req.Msg.GetPageSize()), req.Msg.GetPageToken())
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	out := make([]*saasv1.File, len(page.Files))
	for i, f := range page.Files {
		out[i] = fileMessage(f)
	}
	return connect.NewResponse(&saasv1.ListFilesResponse{
		Files: out, NextPageToken: page.NextToken, UsedBytes: page.UsedBytes, QuotaBytes: page.Quota,
	}), nil
}

func (h *fileHandler) GetDownloadUrl(ctx context.Context, req *connect.Request[saasv1.GetDownloadUrlRequest]) (*connect.Response[saasv1.GetDownloadUrlResponse], error) {
	if req.Msg.GetFileId() == "" {
		return nil, requiredField("file_id")
	}
	if _, err := h.authorize(ctx, req.Msg.GetWorkspaceId(), false); err != nil {
		return nil, err
	}
	d, err := h.files.DownloadURL(ctx, req.Msg.GetWorkspaceId(), req.Msg.GetFileId())
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.GetDownloadUrlResponse{Url: d.URL, ExpiresAt: d.ExpiresAt.UTC().Format(time.RFC3339)}), nil
}

func (h *fileHandler) DeleteFile(ctx context.Context, req *connect.Request[saasv1.DeleteFileRequest]) (*connect.Response[saasv1.DeleteFileResponse], error) {
	if req.Msg.GetFileId() == "" {
		return nil, requiredField("file_id")
	}
	if _, err := h.authorize(ctx, req.Msg.GetWorkspaceId(), true); err != nil {
		return nil, err
	}
	if err := h.files.DeleteFile(ctx, req.Msg.GetWorkspaceId(), req.Msg.GetFileId()); err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.DeleteFileResponse{}), nil
}

// fileMessage is the wire form of a file. The object key stays server-side.
func fileMessage(f core.File) *saasv1.File {
	return &saasv1.File{
		Id: f.ID, WorkspaceId: f.WorkspaceID, Name: f.Name, ContentType: f.ContentType,
		SizeBytes: f.SizeBytes, Status: string(f.Status), CreatedBy: f.CreatedBy,
		CreatedAt: f.CreatedAt.UTC().Format(time.RFC3339),
	}
}
