package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/bernardoforcillo/drops/pg"
)

// store holds the SQL of the background jobs. The table and column names are authlayer's
// defaults (sessions, verifications, organization_invites, organizations; see
// saas.createIdentitySchema), the billing_subscriptions table written by the subscription sink,
// and job_notifications (saas migrations).
type store struct{ db *pg.DB }

var (
	_ invitationPurger = (*store)(nil)
	_ sessionPurger    = (*store)(nil)
	_ trialSource      = (*store)(nil)
	_ notificationLog  = (*store)(nil)
)

func (s *store) exec(ctx context.Context, what, query string, args ...any) (int64, error) {
	res, err := s.db.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", what, err)
	}
	return res.RowsAffected()
}

// DeleteExpiredInvitations removes invitations past their expiry. PendingInvitations and the
// accept flow already refuse them; this only bounds the table.
func (s *store) DeleteExpiredInvitations(ctx context.Context, now time.Time) (int64, error) {
	return s.exec(ctx, "deleting expired invitations", `DELETE FROM organization_invites WHERE expires_at < $1`, now)
}

// DeleteExpiredSessions removes refresh sessions that expired before the cutoff. A session is
// live until expires_at, so with a cutoff in the past nothing live (nor a rotated predecessor
// still inside its lifetime, which authlayer keeps for reuse detection) is deleted.
func (s *store) DeleteExpiredSessions(ctx context.Context, before time.Time) (int64, error) {
	return s.exec(ctx, "deleting expired sessions", `DELETE FROM sessions WHERE expires_at < $1`, before)
}

// DeleteExpiredVerifications removes one-time tokens that expired before the cutoff. authlayer
// deletes a token when it is consumed, so "used" tokens are already gone.
func (s *store) DeleteExpiredVerifications(ctx context.Context, before time.Time) (int64, error) {
	return s.exec(ctx, "deleting expired verifications", `DELETE FROM verifications WHERE expires_at < $1`, before)
}

// EndingTrials lists workspaces whose provider status is trialing and whose period end (the trial
// end while trialing) falls in (from, to]. Workspace ids are compared as text so one malformed id
// in billing_subscriptions cannot fail the whole query.
func (s *store) EndingTrials(ctx context.Context, from, to time.Time) ([]EndingTrial, error) {
	rows, err := s.db.Query(ctx, `
		SELECT b.workspace_id, o.name, b.current_period_end
		FROM billing_subscriptions b JOIN organizations o ON o.id::text = b.workspace_id
		WHERE b.status = 'trialing' AND b.current_period_end > $1 AND b.current_period_end <= $2
		ORDER BY b.current_period_end, b.workspace_id`, from, to)
	if err != nil {
		return nil, fmt.Errorf("reading ending trials: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []EndingTrial
	for rows.Next() {
		var t EndingTrial
		if err := rows.Scan(&t.WorkspaceID, &t.WorkspaceName, &t.EndsAt); err != nil {
			return nil, fmt.Errorf("scanning ending trial: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Claim inserts (kind, key); only the first caller, on any replica, gets true.
func (s *store) Claim(ctx context.Context, kind, key string) (bool, error) {
	n, err := s.exec(ctx, "recording notification",
		`INSERT INTO job_notifications (kind, key) VALUES ($1, $2) ON CONFLICT (kind, key) DO NOTHING`, kind, key)
	return n == 1, err
}

// Release deletes a claim whose message could not be sent, so a retry sends it.
func (s *store) Release(ctx context.Context, kind, key string) error {
	_, err := s.exec(ctx, "releasing notification", `DELETE FROM job_notifications WHERE kind = $1 AND key = $2`, kind, key)
	return err
}
