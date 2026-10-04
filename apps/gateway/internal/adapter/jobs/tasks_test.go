package jobs

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

var now0 = time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)

type fakePurger struct {
	invitationsBefore, sessionsBefore, verificationsBefore time.Time
	sessionsErr, verificationsErr, invitationsErr          error
}

func (f *fakePurger) DeleteExpiredInvitations(_ context.Context, now time.Time) (int64, error) {
	f.invitationsBefore = now
	return 2, f.invitationsErr
}

func (f *fakePurger) DeleteExpiredSessions(_ context.Context, before time.Time) (int64, error) {
	f.sessionsBefore = before
	return 3, f.sessionsErr
}

func (f *fakePurger) DeleteExpiredVerifications(_ context.Context, before time.Time) (int64, error) {
	f.verificationsBefore = before
	return 4, f.verificationsErr
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)) }

func TestInvitationsCleanup_DeletesWhatExpiredBeforeNow(t *testing.T) {
	p := &fakePurger{}
	if err := invitationsCleanup(p, func() time.Time { return now0 }, quietLog())(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !p.invitationsBefore.Equal(now0) {
		t.Errorf("cutoff = %v, want now", p.invitationsBefore)
	}
	p.invitationsErr = errors.New("db down")
	if err := invitationsCleanup(p, func() time.Time { return now0 }, quietLog())(context.Background()); err == nil {
		t.Error("a failed delete must fail the run so it is retried")
	}
}

func TestSessionsCleanup_KeepsAGracePeriodAndTriesBothDeletes(t *testing.T) {
	p := &fakePurger{sessionsErr: errors.New("boom")}
	err := sessionsCleanup(p, func() time.Time { return now0 }, quietLog())(context.Background())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want the sessions error", err)
	}
	if want := now0.Add(-sessionGrace); !p.sessionsBefore.Equal(want) {
		t.Errorf("sessions cutoff = %v, want %v", p.sessionsBefore, want)
	}
	if want := now0.Add(-verificationGrace); !p.verificationsBefore.Equal(want) {
		t.Errorf("a failing first delete must not skip the second; verifications cutoff = %v, want %v", p.verificationsBefore, want)
	}
}

type fakeTrials struct {
	list     []EndingTrial
	from, to time.Time
	err      error
}

func (f *fakeTrials) EndingTrials(_ context.Context, from, to time.Time) ([]EndingTrial, error) {
	f.from, f.to = from, to
	return f.list, f.err
}

type fakeOwners struct {
	owners  map[string][]string // workspace -> owner ids
	emails  map[string]string   // account -> address
	failFor string              // workspace whose owner lookup fails
}

func (f *fakeOwners) OwnerIDs(_ context.Context, ws string) ([]string, error) {
	if ws == f.failFor {
		return nil, errors.New("lookup failed")
	}
	return f.owners[ws], nil
}

func (f *fakeOwners) MemberEmails(_ context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if e, ok := f.emails[id]; ok {
			out[id] = e
		}
	}
	return out, nil
}

type memLog struct{ claimed map[string]bool }

func (m *memLog) Claim(_ context.Context, kind, key string) (bool, error) {
	k := kind + "|" + key
	if m.claimed[k] {
		return false, nil
	}
	m.claimed[k] = true
	return true, nil
}

func (m *memLog) Release(_ context.Context, kind, key string) error {
	delete(m.claimed, kind+"|"+key)
	return nil
}

type sentMail struct{ to, workspace, endsOn, url string }

type fakeMail struct {
	sent    []sentMail
	failFor string // recipient whose send fails
}

func (f *fakeMail) SendTrialEnding(_ context.Context, to, workspace, endsOn, url string) error {
	if to == f.failFor {
		return errors.New("provider rejected the message")
	}
	f.sent = append(f.sent, sentMail{to, workspace, endsOn, url})
	return nil
}

type reminderEnv struct {
	r      *trialReminder
	trials *fakeTrials
	owners *fakeOwners
	mail   *fakeMail
	logs   *bytes.Buffer
}

func newReminder() reminderEnv {
	ends := now0.Add(48 * time.Hour)
	trials := &fakeTrials{list: []EndingTrial{
		{WorkspaceID: "ws-a", WorkspaceName: "Acme", EndsAt: ends},
		{WorkspaceID: "ws-b", WorkspaceName: "Bolt", EndsAt: ends},
	}}
	owners := &fakeOwners{
		owners: map[string][]string{"ws-a": {"u1"}, "ws-b": {"u2", "u3"}},
		emails: map[string]string{"u1": "ada@example.com", "u2": "bo@example.com", "u3": "cy@example.com"},
	}
	mail := &fakeMail{}
	logs := &bytes.Buffer{}
	return reminderEnv{
		r: &trialReminder{
			trials: trials, owners: owners, sent: &memLog{claimed: map[string]bool{}}, mail: mail,
			billingURL: "https://app.example.com/app/billing", now: func() time.Time { return now0 },
			log: slog.New(slog.NewTextHandler(logs, nil)),
		},
		trials: trials, owners: owners, mail: mail, logs: logs,
	}
}

func TestTrialReminder_EmailsEveryOwnerOnceWithTheTrialDetails(t *testing.T) {
	e := newReminder()
	if err := e.r.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.mail.sent) != 3 {
		t.Fatalf("sent %d emails, want 3 (one per owner): %+v", len(e.mail.sent), e.mail.sent)
	}
	first := e.mail.sent[0]
	if first.to != "ada@example.com" || first.workspace != "Acme" || first.endsOn != "5 October 2026" || first.url != "https://app.example.com/app/billing" {
		t.Errorf("first email = %+v", first)
	}
	if !e.trials.from.Equal(now0) || !e.trials.to.Equal(now0.Add(3*24*time.Hour)) {
		t.Errorf("window = %v..%v, want now..now+3d", e.trials.from, e.trials.to)
	}

	// a second run (a retry, or another replica) sends nothing
	if err := e.r.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.mail.sent) != 3 {
		t.Errorf("second run sent %d more emails", len(e.mail.sent)-3)
	}
}

func TestTrialReminder_ANewTrialEndDateIsRemindedAgain(t *testing.T) {
	e := newReminder()
	_ = e.r.run(context.Background())
	for i := range e.trials.list {
		e.trials.list[i].EndsAt = e.trials.list[i].EndsAt.Add(24 * time.Hour) // trial extended
	}
	_ = e.r.run(context.Background())
	if len(e.mail.sent) != 6 {
		t.Errorf("sent %d emails, want 6", len(e.mail.sent))
	}
}

func TestTrialReminder_OneFailureDoesNotStopTheOthersAndIsRetried(t *testing.T) {
	e := newReminder()
	e.owners.failFor = "ws-a"
	e.mail.failFor = "cy@example.com"

	err := e.r.run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ws-a") || !strings.Contains(err.Error(), "ws-b") {
		t.Fatalf("err = %v, want both failures aggregated", err)
	}
	if len(e.mail.sent) != 1 || e.mail.sent[0].to != "bo@example.com" {
		t.Fatalf("sent = %+v, want only bo@ (ws-b's other owner succeeded)", e.mail.sent)
	}
	if strings.Contains(err.Error(), "@") {
		t.Errorf("the error leaks an address: %v", err)
	}

	// everything recovers: only what failed is sent now
	e.owners.failFor, e.mail.failFor = "", ""
	if err := e.r.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range e.mail.sent {
		got = append(got, m.to)
	}
	if strings.Join(got, ",") != "bo@example.com,ada@example.com,cy@example.com" {
		t.Errorf("recipients over both runs = %v: each owner exactly once", got)
	}
}

func TestTrialReminder_SkipsAnOwnerWithoutAnAccount(t *testing.T) {
	e := newReminder()
	delete(e.owners.emails, "u1")
	if err := e.r.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.mail.sent) != 2 {
		t.Errorf("sent %d, want 2", len(e.mail.sent))
	}
}

func TestTrialReminder_FailsTheRunWhenTrialsCannotBeRead(t *testing.T) {
	e := newReminder()
	e.trials.err = errors.New("db down")
	if err := e.r.run(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestTrialReminder_LogsCountsNotAddresses(t *testing.T) {
	e := newReminder()
	_ = e.r.run(context.Background())
	out := e.logs.String()
	if !strings.Contains(out, "sent=3") || strings.Contains(out, "@") {
		t.Errorf("logs = %s", out)
	}
}
