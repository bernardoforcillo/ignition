package saas

import (
	"context"
	"fmt"

	alauth "github.com/bernardoforcillo/authlayer/auth"
	"github.com/bernardoforcillo/drops/pg"

	"github.com/bernardoforcillo/ignition/go-packages/identity/account"
)

// directory is the SQL the identity engines do not offer: workspace erasure and
// invitations by address. The table names are authlayer's defaults (see createIdentitySchema)
// and the features/billing tables of this package.
type directory struct{ db *pg.DB }

var _ account.Store = (*directory)(nil)

// blockingStatuses are the provider states in which the workspace is still being billed.
var blockingStatuses = []string{"active", "trialing", "past_due"}

// CanErase refuses account.ErrActiveSubscription while the workspace is still billed: erasing it
// would leave the payment provider charging a workspace that no longer exists.
func (d *directory) CanErase(ctx context.Context, workspaceID string) error {
	rows, err := d.db.Query(ctx, `SELECT status FROM billing_subscriptions WHERE workspace_id = $1`, workspaceID)
	if err != nil {
		return fmt.Errorf("reading billing state: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return rows.Err()
	}
	var status string
	if err := rows.Scan(&status); err != nil {
		return fmt.Errorf("scanning billing state: %w", err)
	}
	for _, s := range blockingStatuses {
		if status == s {
			return account.ErrActiveSubscription
		}
	}
	return nil
}

// EraseWorkspace deletes the workspace and everything keyed on it in one transaction. Deleting
// what is already gone is not an error.
func (d *directory) EraseWorkspace(ctx context.Context, workspaceID string) error {
	steps := []string{
		`DELETE FROM organization_invites WHERE container_id = $1::uuid`,
		`DELETE FROM organization_invite_links WHERE container_id = $1::uuid`,
		`DELETE FROM organization_roles WHERE container_id = $1::uuid`,
		`DELETE FROM organization_members WHERE container_id = $1::uuid`,
		`DELETE FROM organizations WHERE id = $1::uuid`,
		`DELETE FROM feature_usage WHERE tenant_id = $1`,
		`DELETE FROM feature_subscriptions WHERE tenant_id = $1`,
		`DELETE FROM billing_subscriptions WHERE workspace_id = $1`,
		`DELETE FROM billing_customers WHERE workspace_id = $1`,
	}
	return d.db.InTx(ctx, func(tx *pg.DB) error {
		for _, q := range steps {
			if _, err := tx.Exec(ctx, q, workspaceID); err != nil {
				return fmt.Errorf("erasing workspace: %w", err)
			}
		}
		return nil
	})
}

// PendingInvitations lists the unexpired invitations addressed to email.
func (d *directory) PendingInvitations(ctx context.Context, email string) ([]account.PendingInvitation, error) {
	rows, err := d.db.Query(ctx, `
		SELECT o.name, i.role_key, i.created_at, i.expires_at
		FROM organization_invites i JOIN organizations o ON o.id = i.container_id
		WHERE i.email = $1 AND i.expires_at > now() ORDER BY i.created_at`,
		alauth.NormalizeEmail(email))
	if err != nil {
		return nil, fmt.Errorf("reading invitations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []account.PendingInvitation
	for rows.Next() {
		var inv account.PendingInvitation
		if err := rows.Scan(&inv.WorkspaceName, &inv.RoleKey, &inv.CreatedAt, &inv.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scanning invitation: %w", err)
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// DeleteInvitations removes every invitation addressed to email.
func (d *directory) DeleteInvitations(ctx context.Context, email string) error {
	if _, err := d.db.Exec(ctx, `DELETE FROM organization_invites WHERE email = $1`, alauth.NormalizeEmail(email)); err != nil {
		return fmt.Errorf("deleting invitations: %w", err)
	}
	return nil
}
