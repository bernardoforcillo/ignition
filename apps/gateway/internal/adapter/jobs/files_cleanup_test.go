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

type fakeFilesPurger struct {
	n     int
	err   error
	calls int
}

func (f *fakeFilesPurger) PurgeAbandoned(context.Context) (int, error) {
	f.calls++
	return f.n, f.err
}

func TestFilesCleanup_RunsThePurgeAndLogsOnlyTheCount(t *testing.T) {
	var logs bytes.Buffer
	p := &fakeFilesPurger{n: 3}
	if err := filesCleanup(p, slog.New(slog.NewTextHandler(&logs, nil)))(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.calls != 1 || !strings.Contains(logs.String(), "count=3") {
		t.Errorf("calls = %d, log = %q", p.calls, logs.String())
	}
}

func TestFilesCleanup_APartialFailureFailsTheRunSoItIsRetried(t *testing.T) {
	boom := errors.New("storage down")
	p := &fakeFilesPurger{n: 1, err: boom}
	if err := filesCleanup(p, quietLog())(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the purge error", err)
	}
}

func TestTasks_FilesCleanupIsScheduledOnlyWithObjectStorage(t *testing.T) {
	names := func(d Deps) map[string]time.Duration {
		out := map[string]time.Duration{}
		for _, task := range tasks(d, 0) {
			out[task.Name] = task.Every
		}
		return out
	}
	d := Deps{Logger: quietLog(), Now: func() time.Time { return now0 }}
	if _, ok := names(d)[FilesCleanup]; ok {
		t.Error("files.cleanup must not be scheduled when storage is off")
	}
	d.Files = &fakeFilesPurger{}
	if every, ok := names(d)[FilesCleanup]; !ok || every != 15*time.Minute {
		t.Errorf("files.cleanup = %v, %v, want every 15m", every, ok)
	}
}
