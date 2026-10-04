package saas

import (
	"errors"
	"testing"
	"time"

	featurelayer "github.com/bernardoforcillo/featurelayer"
	"github.com/bernardoforcillo/featurelayer/entitlement"
	"github.com/bernardoforcillo/featurelayer/flags"

	"github.com/bernardoforcillo/ignition/go-packages/features"
)

func quotaFor(t *testing.T, sub *entitlement.Subscription, cfg *featurelayer.Config) (int64, error) {
	t.Helper()
	subs := entitlement.NewMemSubscriptions()
	if sub != nil {
		subs.Set(*sub)
	}
	var opts []features.Option
	if cfg != nil {
		opts = append(opts, features.WithConfig(*cfg))
	}
	engine, err := features.New(subs, entitlement.NewMemUsage(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	return storageQuota{engine: engine}.Limit(t.Context(), "ws-1", "u1")
}

func TestStorageQuota_FollowsThePlan(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for plan, want := range map[entitlement.PlanID]int64{
		features.PlanFree: features.FreeStorageBytes,
		features.PlanPro:  features.ProStorageBytes,
	} {
		got, err := quotaFor(t, &entitlement.Subscription{TenantID: "ws-1", Plan: plan, BillingAnchor: anchor}, nil)
		if err != nil || got != want {
			t.Errorf("%s: quota = %d, %v, want %d", plan, got, err, want)
		}
	}
}

func TestStorageQuota_FailsClosed(t *testing.T) {
	if _, err := quotaFor(t, nil, nil); !errors.Is(err, features.ErrNotEntitled) {
		t.Errorf("a workspace with no subscription: err = %v, want ErrNotEntitled", err)
	}

	// The kill switch: a flag turned off refuses storage with no deploy.
	cfg := features.Config()
	cfg.Flags = append(cfg.Flags, flags.Flag{Feature: features.StorageBytes, Enabled: false})
	sub := &entitlement.Subscription{TenantID: "ws-1", Plan: features.PlanFree, BillingAnchor: time.Now()}
	if _, err := quotaFor(t, sub, &cfg); !errors.Is(err, features.ErrDisabled) {
		t.Errorf("a switched-off feature: err = %v, want ErrDisabled", err)
	}
}

func TestStorageQuota_ZeroAllowanceIsAFullQuota(t *testing.T) {
	cfg := features.Config()
	for i, p := range cfg.Plans {
		if p.ID != features.PlanFree {
			continue
		}
		ents := append([]entitlement.Entitlement(nil), p.Entitlements...)
		for j, e := range ents {
			if e.Feature == features.StorageBytes {
				ents[j] = entitlement.Limited(features.StorageBytes, 0, entitlement.None)
			}
		}
		cfg.Plans[i].Entitlements = ents
	}
	sub := &entitlement.Subscription{TenantID: "ws-1", Plan: features.PlanFree, BillingAnchor: time.Now()}
	if _, err := quotaFor(t, sub, &cfg); !errors.Is(err, features.ErrLimitReached) {
		t.Errorf("a plan with no storage: err = %v, want ErrLimitReached", err)
	}
}
