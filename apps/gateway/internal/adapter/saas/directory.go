package saas

import (
	"context"
	"fmt"
	"strings"

	alauth "github.com/bernardoforcillo/authlayer/auth"
	"github.com/bernardoforcillo/drops/pg"

	"github.com/bernardoforcillo/ignition/go-packages/identity/account"
)

// directory is the SQL the identity engines do not offer: member emails, workspace erasure and
// invitations by address. The table names are authlayer's defaults (see createIdentitySchema)
// and the features/billing tables of this package.
type directory struct{ db *pg.DB }

var _ account.Store = (*directory)(nil)

// Emails returns the address of each user id it can resolve; unknown ids are simply absent.
func (d *directory) Emails(ctx context.Context, userIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	holders := make([]string, len(userIDs))
	args := make([]any, len(userIDs))
	for i, id := range userIDs {
		holders[i] = fmt.Sprintf("$%d::uuid", i+1)
		args[i] = id
	}
	rows, err := d.db.Query(ctx, `SELECT id::text, email FROM users WHERE id IN (`+strings.Join(holders, ",")+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("reading member emails: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, email string
		if err := rows.Scan(&id, &email); err != nil {
			return nil, fmt.Errorf("scanning member email: %w", err)
		}
		out[id] = email
	}
	return out, rows.Err()
}

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
