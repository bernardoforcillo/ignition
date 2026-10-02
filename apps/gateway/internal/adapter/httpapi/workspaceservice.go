package httpapi

import (
	"context"

	"connectrpc.com/connect"

	"github.com/bernardoforcillo/ignition/go-packages/identity/workspace"

	saasv1 "github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1"
)

// workspaceHandler implements saasv1connect.WorkspaceServiceHandler. The
// workspace service itself enforces membership and the role permissions
// (invite needs the invite permission); this handler authenticates via
// the interceptor's subject and delegates.
type workspaceHandler struct{ workspaces workspaceService }

func (h *workspaceHandler) ListWorkspaces(ctx context.Context, _ *connect.Request[saasv1.ListWorkspacesRequest]) (*connect.Response[saasv1.ListWorkspacesResponse], error) {
	user, err := subjectOrErr(ctx)
	if err != nil {
		return nil, err
	}
	memberships, err := h.workspaces.ListFor(ctx, user)
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	out := make([]*saasv1.WorkspaceMembership, len(memberships))
	for i, m := range memberships {
		out[i] = &saasv1.WorkspaceMembership{Workspace: workspaceMessage(m.Workspace), RoleKey: m.RoleKey}
	}
	return connect.NewResponse(&saasv1.ListWorkspacesResponse{Workspaces: out}), nil
}

func (h *workspaceHandler) CreateWorkspace(ctx context.Context, req *connect.Request[saasv1.CreateWorkspaceRequest]) (*connect.Response[saasv1.CreateWorkspaceResponse], error) {
	user, err := subjectOrErr(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetName() == "" {
		return nil, requiredField("name")
	}
	ws, err := h.workspaces.Create(ctx, user, req.Msg.GetName(), req.Msg.GetSlug())
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.CreateWorkspaceResponse{Workspace: workspaceMessage(ws)}), nil
}

func (h *workspaceHandler) GetWorkspace(ctx context.Context, req *connect.Request[saasv1.GetWorkspaceRequest]) (*connect.Response[saasv1.GetWorkspaceResponse], error) {
	user, err := subjectOrErr(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetWorkspaceId() == "" {
		return nil, requiredField("workspace_id")
	}
	ws, err := h.workspaces.Get(ctx, user, req.Msg.GetWorkspaceId())
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.GetWorkspaceResponse{Workspace: workspaceMessage(ws)}), nil
}

func (h *workspaceHandler) ListMembers(ctx context.Context, req *connect.Request[saasv1.ListMembersRequest]) (*connect.Response[saasv1.ListMembersResponse], error) {
	user, err := subjectOrErr(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetWorkspaceId() == "" {
		return nil, requiredField("workspace_id")
	}
	members, err := h.workspaces.ListMembers(ctx, user, req.Msg.GetWorkspaceId())
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	out := make([]*saasv1.Member, len(members))
	for i, m := range members {
		out[i] = &saasv1.Member{UserId: m.UserID, RoleKey: m.RoleKey}
	}
	return connect.NewResponse(&saasv1.ListMembersResponse{Members: out}), nil
}

func (h *workspaceHandler) InviteMember(ctx context.Context, req *connect.Request[saasv1.InviteMemberRequest]) (*connect.Response[saasv1.InviteMemberResponse], error) {
	user, err := subjectOrErr(ctx)
	if err != nil {
		return nil, err
	}
	switch {
	case req.Msg.GetWorkspaceId() == "":
		return nil, requiredField("workspace_id")
	case req.Msg.GetEmail() == "":
		return nil, requiredField("email")
	case req.Msg.GetRoleKey() == "":
		return nil, requiredField("role_key")
	}
	inv, err := h.workspaces.Invite(ctx, user, req.Msg.GetWorkspaceId(), req.Msg.GetEmail(), req.Msg.GetRoleKey())
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.InviteMemberResponse{
		Invitation: &saasv1.Invitation{Id: inv.ID, Email: inv.Email, RoleKey: inv.RoleKey},
	}), nil
}

func (h *workspaceHandler) AcceptInvite(ctx context.Context, req *connect.Request[saasv1.AcceptInviteRequest]) (*connect.Response[saasv1.AcceptInviteResponse], error) {
	user, err := subjectOrErr(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetToken() == "" {
		return nil, requiredField("token")
	}
	ws, err := h.workspaces.AcceptInvite(ctx, user, req.Msg.GetToken())
	if err != nil {
		return nil, toConnectError(ctx, err)
	}
	return connect.NewResponse(&saasv1.AcceptInviteResponse{Workspace: workspaceMessage(ws)}), nil
}

func workspaceMessage(w workspace.Workspace) *saasv1.Workspace {
	return &saasv1.Workspace{Id: w.ID, Name: w.Name, Slug: w.Slug}
}

// subjectOrErr returns the authenticated user, or Unauthenticated when
// the interceptor did not run (a wiring bug: fail closed).
func subjectOrErr(ctx context.Context) (string, error) {
	id, ok := subjectFrom(ctx)
	if !ok {
		return "", connect.NewError(connect.CodeUnauthenticated, errUnauthenticated)
	}
	return id, nil
}
