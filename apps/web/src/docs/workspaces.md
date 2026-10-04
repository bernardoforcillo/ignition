# Workspaces and roles

A workspace is the unit your customers work in. It owns its members, its invitations and its plan.

## Roles

Every workspace has three roles. They exist without any setup.

| Role | What it can do |
| --- | --- |
| Owner | Everything, including deleting the workspace and transferring ownership |
| Admin | Everything except deleting the workspace, including inviting people and managing billing |
| Member | See the workspace and its member list, and nothing beyond that |

Every workspace has exactly one owner. The owner cannot be removed or demoted, and cannot leave without first transferring ownership. You can only hand out a role that is no more powerful than your own.

Billing is limited on purpose: starting a checkout and opening the customer portal need permission to update the workspace, which owners and admins have.

Roles are checked on the server for every call. Someone who is not a member of a workspace gets "not found", so a workspace's existence is never revealed to outsiders.

## Inviting people

Owners and admins invite people from the Members page:

1. Enter the person's email address and pick Member or Admin.
2. Ignition emails them a link that carries a single-use token.
3. They sign in, or create an account first, and open the link to join with the role you chose.

Inviting an address that already has a pending invitation replaces the old one. An invitation that is not accepted in time expires, and the person is told to ask for a new one.

## Several workspaces

An account can belong to any number of workspaces, with a different role in each. The workspace switcher in the app changes which one you are looking at. Plans, limits and billing belong to the workspace, not to your account.
