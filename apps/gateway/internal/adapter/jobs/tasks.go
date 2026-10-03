package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	jobslib "github.com/bernardoforcillo/ignition/go-packages/jobs"
)

// Task names, the keys of the schedule in scheduled_job_runs: renaming one starts a new schedule.
const (
	InvitationsCleanup = "invitations.cleanup"
	SessionsCleanup    = "sessions.cleanup"
	TrialsRemind       = "trials.remind"
)

const (
	// A refresh session or verification token is only deleted well after it expired: an expired
	// row is already refused by authlayer, the grace just keeps recent history for diagnosis.
	sessionGrace      = 7 * 24 * time.Hour
	verificationGrace = 24 * time.Hour

	// A trial is reminded about when it ends within this window.
	trialWarning = 3 * 24 * time.Hour

	trialNotificationKind = "trial-ending"
)

// The interfaces below are what each task needs, declared here and implemented by store (SQL),
// saas.Services (owners and addresses) and the mailer, so a task is tested with fakes.

type invitationPurger interface {
	DeleteExpiredInvitations(ctx context.Context, now time.Time) (int64, error)
}

type sessionPurger interface {
	DeleteExpiredSessions(ctx context.Context, before time.Time) (int64, error)
	DeleteExpiredVerifications(ctx context.Context, before time.Time) (int64, error)
}

// EndingTrial is a workspace whose provider trial ends inside the reminder window.
type EndingTrial struct {
	WorkspaceID   string
	WorkspaceName string
	EndsAt        time.Time
}

type trialSource interface {
	EndingTrials(ctx context.Context, from, to time.Time) ([]EndingTrial, error)
}

// OwnerDirectory resolves who to write to. saas.Services implements it.
type OwnerDirectory interface {
	// OwnerIDs lists the account ids holding the owner role of the workspace.
	OwnerIDs(ctx context.Context, workspaceID string) ([]string, error)
	// MemberEmails resolves account ids to addresses; unknown ids are absent.
	MemberEmails(ctx context.Context, ids []string) (map[string]string, error)
}

// TrialMailer sends the reminder. *mailer.Mailer implements it.
type TrialMailer interface {
	SendTrialEnding(ctx context.Context, to, workspaceName, endsOn, url string) error
}

// notificationLog makes a notification at-most-once across retries and replicas: Claim is an
// atomic insert that reports whether this caller is the first for (kind, key).
type notificationLog interface {
	Claim(ctx context.Context, kind, key string) (bool, error)
	Release(ctx context.Context, kind, key string) error
}

// invitationsCleanup deletes invitations that expired; only a count is logged.
func invitationsCleanup(p invitationPurger, now func() time.Time, log *slog.Logger) func(context.Context) error {
	return func(ctx context.Context) error {
		n, err := p.DeleteExpiredInvitations(ctx, now())
		if err != nil {
			return err
		}
		log.InfoContext(ctx, "expired invitations deleted", "count", n)
		return nil
	}
}

// sessionsCleanup deletes refresh sessions and verification tokens that expired more than their
// grace period ago. A live session (not yet expired) is never touched. Both deletes are tried
// even if the first fails.
func sessionsCleanup(p sessionPurger, now func() time.Time, log *slog.Logger) func(context.Context) error {
	return func(ctx context.Context) error {
		t := now()
		sessions, err1 := p.DeleteExpiredSessions(ctx, t.Add(-sessionGrace))
		verifications, err2 := p.DeleteExpiredVerifications(ctx, t.Add(-verificationGrace))
		log.InfoContext(ctx, "expired sessions and tokens deleted", "sessions", sessions, "verifications", verifications)
		return errors.Join(err1, err2)
	}
}

// trialReminder is the trials.remind task. It emails each owner of a workspace whose trial ends
// within three days, once per owner and trial end date.
type trialReminder struct {
	trials trialSource
	owners OwnerDirectory
	sent   notificationLog
	mail   TrialMailer
	// billingURL is where the email sends the owner.
	billingURL string
	now        func() time.Time
	log        *slog.Logger
}

func (r *trialReminder) run(ctx context.Context) error {
	now := r.now()
	trials, err := r.trials.EndingTrials(ctx, now, now.Add(trialWarning))
	if err != nil {
		return err
	}
	var errs []error
	var sent, already int
	for _, trial := range trials {
		s, a, err := r.remind(ctx, trial)
		sent, already = sent+s, already+a
		if err != nil {
			// One workspace failing must not stop the others; the run still ends as failed so
			// the scheduler retries, and the log claims make that retry skip what was sent.
			errs = append(errs, fmt.Errorf("workspace %s: %w", trial.WorkspaceID, err))
		}
	}
	r.log.InfoContext(ctx, "trial reminders processed", "trials", len(trials), "sent", sent, "already_sent", already, "failed", len(errs))
	return errors.Join(errs...)
}

// remind writes to every owner of one workspace and reports how many mails it sent and how many
// owners had already been told.
func (r *trialReminder) remind(ctx context.Context, trial EndingTrial) (sent, already int, err error) {
	ids, err := r.owners.OwnerIDs(ctx, trial.WorkspaceID)
	if err != nil {
		return 0, 0, fmt.Errorf("resolving owners: %w", err)
	}
	emails, err := r.owners.MemberEmails(ctx, ids)
	if err != nil {
		return 0, 0, fmt.Errorf("resolving owner addresses: %w", err)
	}
	endsOn := trial.EndsAt.UTC().Format("2 January 2006")
	var errs []error
	for _, id := range ids {
		to, ok := emails[id]
		if !ok {
			continue // account deleted since
		}
		key := strings.Join([]string{trial.WorkspaceID, trial.EndsAt.UTC().Format(time.DateOnly), id}, ":")
		first, err := r.sent.Claim(ctx, trialNotificationKind, key)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !first {
			already++
			continue
		}
		if err := r.mail.SendTrialEnding(ctx, to, trial.WorkspaceName, endsOn, r.billingURL); err != nil {
			// Give the claim back so the retry sends it; the error is wrapped without the address.
			errs = append(errs, fmt.Errorf("sending reminder: %w", err))
			if rerr := r.sent.Release(ctx, trialNotificationKind, key); rerr != nil {
				errs = append(errs, rerr)
			}
			continue
		}
		sent++
	}
	return sent, already, errors.Join(errs...)
}

// tasks builds the task list over the three collaborators' implementations.
func tasks(d Deps, every time.Duration) []jobslib.Task {
	st := &store{db: d.DB}
	log := d.Logger
	interval := func(def time.Duration) time.Duration {
		if every > 0 {
			return every
		}
		return def
	}
	reminder := &trialReminder{
		trials: st, owners: d.Owners, sent: st, mail: d.Mail,
		billingURL: strings.TrimRight(d.AppURL, "/") + "/app/billing", now: d.Now, log: log,
	}
	return []jobslib.Task{
		{Name: InvitationsCleanup, Every: interval(time.Hour), Run: invitationsCleanup(st, d.Now, log)},
		{Name: SessionsCleanup, Every: interval(24 * time.Hour), Run: sessionsCleanup(st, d.Now, log)},
		{Name: TrialsRemind, Every: interval(24 * time.Hour), Run: reminder.run},
	}
}
