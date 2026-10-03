// Package permissions is the template's permission surface: the one place that
// says which (resource, action) pairs exist. authlayer enforces them; nothing
// outside this set can be granted or checked.
//
// The workspace control resources (workspace, member, role, invite) come from
// authlayer's scope.ControlStatements and are merged in by NewAccess, so only
// product resources are declared here.
package permissions

import (
	"github.com/bernardoforcillo/authlayer/access"
	"github.com/bernardoforcillo/authlayer/org"
	"github.com/bernardoforcillo/authlayer/scope"
)

// Resources and actions, re-exported so call sites read in one vocabulary.
const (
	ResourceWorkspace = org.ResourceOrganization
	ResourceMember    = scope.ResourceMember
	ResourceRole      = scope.ResourceRole
	ResourceInvite    = scope.ResourceInvite

	ActionCreate = scope.ActionCreate
	ActionRead   = scope.ActionRead
	ActionUpdate = scope.ActionUpdate
	ActionDelete = scope.ActionDelete

	// ActionWrite covers uploading and deleting files: ResourceFile has two permissions, read and write.
	ActionWrite access.Action = "write"
)

// Default role keys. They exist in every workspace without a stored row: owner
// holds everything, admin everything except deleting the workspace, member
// nothing beyond membership. Grant members more through custom roles.
const (
	RoleOwner  = scope.RoleOwner
	RoleAdmin  = scope.RoleAdmin
	RoleMember = scope.RoleMember
)

// ResourceProject is an example product resource.
//
// ADD PRODUCT RESOURCES HERE: declare a constant and list its actions in
// Statements. Delete this example once the first real resource exists.
const ResourceProject = "project"

// ResourceFile is the workspace's uploaded files: file:read lists and downloads, file:write
// uploads and deletes. Owners and admins hold both; grant members through a custom role.
const ResourceFile = "file"

// Statements is the product-specific part of the permission surface.
func Statements() map[string][]access.Action {
	return map[string][]access.Action{
		// ADD PRODUCT RESOURCES HERE.
		ResourceProject: {ActionCreate, ActionRead, ActionUpdate, ActionDelete},
		ResourceFile:    {ActionRead, ActionWrite},
	}
}

// NewAccess builds the access engine for workspaces. Build it once and share
// it: permissions from two engines cannot be compared with each other.
func NewAccess() *access.Access { return org.NewAccess(Statements()) }
