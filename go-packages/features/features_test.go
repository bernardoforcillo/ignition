package features_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	featurelayer "github.com/bernardoforcillo/featurelayer"
	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"

	"github.com/bernardoforcillo/ignition/go-packages/features"
)

const ws = "ws-1"

var t0 = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

// failingSubs is a hand-written SubscriptionStore that always errors.
type failingSubs struct{}

func (failingSubs) Subscription(context.Context, string) (*entitlement.Subscription, error) {
	return nil, errors.New("db down")
}

type fixture struct {
	engine *features.Engine
	subs   *entitlement.MemSubscriptions
	now    *time.Time
}

func newFixture(t *testing.T, opts ...features.Option) fixture {
	t.Helper()
	subs := entitlement.NewMemSubscriptions()
	now := t0
	opts = append([]features.Option{features.WithClock(func() time.Time { return now })}, opts...)
	e, err := features.New(subs, entitlement.NewMemUsage(), opts...)
	if err != nil {
		t.Fatalf("features.New: %v", err)
	}
	return fixture{engine: e, subs: subs, now: &now}
}

func (f fixture) subscribe(plan entitlement.PlanID, mut ...func(*entitlement.Subscription)) {
	sub := entitlement.Subscription{TenantID: ws, Plan: plan, BillingAnchor: t0.AddDate(0, -1, 0)}
	for _, m := range mut {
		m(&sub)
	}
	f.subs.Set(sub)
}

func TestConfig_IsValid(t *testing.T) {
	if _, err := featurelayer.NewSnapshot(features.Config()); err != nil {
		t.Fatalf("default catalog is invalid: %v", err)
	}
}

func TestNew_RejectsInvalidDefinitions(t *testing.T) {
	cfg := features.Config()
	cfg.Plans[1].Extends = "nope"
	_, err := features.New(entitlement.NewMemSubscriptions(), entitlement.NewMemUsage(), features.WithConfig(cfg))
	if err == nil {
		t.Fatal("want an error for a plan extending an unknown plan")
	}
}

func TestNew_RequiresBothStores(t *testing.T) {
	if _, err := features.New(nil, entitlement.NewMemUsage()); err == nil {
		t.Fatal("want an error without a subscription store")
	}
	if _, err := features.New(entitlement.NewMemSubscriptions(), nil); err == nil {
		t.Fatal("want an error without a usage store")
	}
}

func TestEvaluate_PlanInheritance(t *testing.T) {
	tests := []struct {
		name string
		plan entitlement.PlanID
		key  catalog.Key
		want bool
	}{
		{"free carries inherited-from feature", features.PlanFree, features.APICalls, true},
		{"free lacks pro feature", features.PlanFree, features.DataExport, false},
		{"pro inherits free's feature", features.PlanPro, features.APICalls, true},
		{"pro has its own feature", features.PlanPro, features.DataExport, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.subscribe(tc.plan)
			if got := f.engine.Allowed(context.Background(), tc.key, ws, "u-1"); got != tc.want {
				t.Fatalf("Allowed = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEvaluate_TrialUnlocksProUntilItEnds(t *testing.T) {
	f := newFixture(t)
	f.subscribe(features.PlanFree, func(s *entitlement.Subscription) {
		s.Trial = &entitlement.PlanTrial{Plan: features.PlanPro, Until: t0.Add(24 * time.Hour)}
	})
	ctx := context.Background()
	if !f.engine.Allowed(ctx, features.DataExport, ws, "u-1") {
		t.Fatal("trial must unlock the pro feature")
	}
	*f.now = t0.Add(48 * time.Hour)
	if f.engine.Allowed(ctx, features.DataExport, ws, "u-1") {
		t.Fatal("feature must lock again once the trial is over")
	}
}

func TestUsage_AddOnRequiresItsPlan(t *testing.T) {
	withAddOn := func(s *entitlement.Subscription) {
		s.AddOns = []entitlement.AddOnID{features.AddOnExtraAPICalls}
	}
	tests := []struct {
		name string
		plan entitlement.PlanID
		want int64
	}{
		{"pro adds the add-on's limit", features.PlanPro, features.ProAPICalls + features.ExtraAPICalls},
		{"free ignores the add-on", features.PlanFree, features.FreeAPICalls},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.subscribe(tc.plan, withAddOn)
			d, err := f.engine.Usage(context.Background(), features.APICalls, ws, "u-1")
			if err != nil {
				t.Fatalf("Usage: %v", err)
			}
			if d.Usage.Max != tc.want {
				t.Fatalf("max = %d, want %d", d.Usage.Max, tc.want)
			}
		})
	}
}

func TestConsume_StopsAtTheLimitAndResetsNextPeriod(t *testing.T) {
	f := newFixture(t)
	f.subscribe(features.PlanFree)
	ctx := context.Background()

	d, err := f.engine.Consume(ctx, features.APICalls, ws, "u-1", features.FreeAPICalls)
	if err != nil || !d.Enabled || d.Usage.Remaining != 0 {
		t.Fatalf("filling the budget: decision=%+v err=%v", d, err)
	}
	d, err = f.engine.Consume(ctx, features.APICalls, ws, "u-1", 1)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if d.Enabled || d.Reason != featurelayer.ReasonLimitReached {
		t.Fatalf("over the limit: enabled=%v reason=%q", d.Enabled, d.Reason)
	}
	if d.Usage.Used != features.FreeAPICalls {
		t.Fatalf("used = %d, a refusal must spend nothing", d.Usage.Used)
	}

	*f.now = d.Usage.ResetsAt.Add(time.Minute)
	d, err = f.engine.Consume(ctx, features.APICalls, ws, "u-1", 1)
	if err != nil || !d.Enabled || d.Usage.Used != 1 {
		t.Fatalf("after reset: decision=%+v err=%v", d, err)
	}
}

func TestEvaluate_KillSwitchFlag(t *testing.T) {
	cfg := features.Config()
	cfg.Flags[0].Enabled = false
	f := newFixture(t, features.WithConfig(cfg))
	f.subscribe(features.PlanPro)

	d := f.engine.Evaluate(context.Background(), features.APICalls, ws, "u-1")
	if d.Enabled || d.Reason != featurelayer.ReasonFlagOff {
		t.Fatalf("enabled=%v reason=%q, want flag_off", d.Enabled, d.Reason)
	}
}

func TestEvaluate_FailsClosed(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown feature", func(t *testing.T) {
		f := newFixture(t)
		f.subscribe(features.PlanPro)
		if f.engine.Allowed(ctx, "no.such.feature", ws, "u-1") {
			t.Fatal("unknown feature must be off")
		}
	})
	t.Run("no subscription", func(t *testing.T) {
		f := newFixture(t)
		if f.engine.Allowed(ctx, features.APICalls, ws, "u-1") {
			t.Fatal("tenant without a subscription must be entitled to nothing")
		}
	})
	t.Run("subscription store error", func(t *testing.T) {
		e, err := features.New(failingSubs{}, entitlement.NewMemUsage())
		if err != nil {
			t.Fatal(err)
		}
		if e.Allowed(ctx, features.APICalls, ws, "u-1") {
			t.Fatal("a store failure must deny")
		}
		err = e.Require(ctx, features.APICalls, ws, "u-1")
		if err == nil || errors.Is(err, features.ErrNotEntitled) || errors.Is(err, features.ErrDisabled) || errors.Is(err, features.ErrLimitReached) {
			t.Fatalf("Require = %v, want an infrastructure error matching no sentinel", err)
		}
	})
}

func TestRequire_MapsDecisionsToSentinels(t *testing.T) {
	ctx := context.Background()
	killed := features.Config()
	killed.Flags[0].Enabled = false

	tests := []struct {
		name  string
		setup func(t *testing.T) *features.Engine
		key   catalog.Key
		want  error
	}{
		{"entitled", func(t *testing.T) *features.Engine {
			f := newFixture(t)
			f.subscribe(features.PlanPro)
			return f.engine
		}, features.DataExport, nil},
		{"plan lacks the feature", func(t *testing.T) *features.Engine {
			f := newFixture(t)
			f.subscribe(features.PlanFree)
			return f.engine
		}, features.DataExport, features.ErrNotEntitled},
		{"no subscription", func(t *testing.T) *features.Engine {
			return newFixture(t).engine
		}, features.APICalls, features.ErrNotEntitled},
		{"switched off", func(t *testing.T) *features.Engine {
			f := newFixture(t, features.WithConfig(killed))
			f.subscribe(features.PlanPro)
			return f.engine
		}, features.APICalls, features.ErrDisabled},
		{"unknown feature", func(t *testing.T) *features.Engine {
			f := newFixture(t)
			f.subscribe(features.PlanPro)
			return f.engine
		}, "no.such.feature", features.ErrDisabled},
		{"limit used up", func(t *testing.T) *features.Engine {
			f := newFixture(t)
			f.subscribe(features.PlanFree)
			if _, err := f.engine.Consume(ctx, features.APICalls, ws, "u-1", features.FreeAPICalls); err != nil {
				t.Fatal(err)
			}
			return f.engine
		}, features.APICalls, features.ErrLimitReached},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.setup(t).Require(ctx, tc.key, ws, "u-1")
			if tc.want == nil {
				if err != nil {
					t.Fatalf("Require = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("Require = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRequireConsume_SpendsOnlyWhenAllowed(t *testing.T) {
	f := newFixture(t)
	f.subscribe(features.PlanFree)
	ctx := context.Background()

	if err := f.engine.RequireConsume(ctx, features.APICalls, ws, "u-1", features.FreeAPICalls); err != nil {
		t.Fatalf("RequireConsume: %v", err)
	}
	err := f.engine.RequireConsume(ctx, features.APICalls, ws, "u-1", 1)
	if !errors.Is(err, features.ErrLimitReached) {
		t.Fatalf("RequireConsume = %v, want ErrLimitReached", err)
	}
	d, err := f.engine.Usage(ctx, features.APICalls, ws, "u-1")
	if err != nil || d.Usage.Used != features.FreeAPICalls {
		t.Fatalf("usage=%+v err=%v, a refusal must not spend", d.Usage, err)
	}
}

func TestWithLogger_LogsDenialsOnly(t *testing.T) {
	var n int
	h := countingHandler{n: &n}
	f := newFixture(t, features.WithLogger(slog.New(h)))
	f.subscribe(features.PlanFree)
	ctx := context.Background()

	f.engine.Evaluate(ctx, features.APICalls, ws, "u-1")
	if n != 0 {
		t.Fatalf("an allowed decision logged %d records", n)
	}
	f.engine.Evaluate(ctx, features.DataExport, ws, "u-1")
	if n != 1 {
		t.Fatalf("a denial logged %d records, want 1", n)
	}
}

// countingHandler is a hand-written slog.Handler that counts records.
type countingHandler struct{ n *int }

func (countingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h countingHandler) Handle(context.Context, slog.Record) error {
	*h.n++
	return nil
}
func (h countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h countingHandler) WithGroup(string) slog.Handler      { return h }

func TestStorageBytes_LimitFollowsThePlanAndNeverResets(t *testing.T) {
	for plan, want := range map[entitlement.PlanID]int64{
		features.PlanFree: features.FreeStorageBytes,
		features.PlanPro:  features.ProStorageBytes,
	} {
		f := newFixture(t)
		f.subscribe(plan)
		d := f.engine.Evaluate(context.Background(), features.StorageBytes, ws, "u-1")
		if !d.Enabled || d.Entitlement == nil || d.Entitlement.Limit == nil {
			t.Fatalf("%s: decision = %+v, want a limited entitlement", plan, d)
		}
		if got := d.Entitlement.Limit; got.Max != want || got.Period != entitlement.None {
			t.Errorf("%s: limit = %+v, want %d bytes with no period", plan, got, want)
		}
	}
}
