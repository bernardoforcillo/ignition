package features

import (
	featurelayer "github.com/bernardoforcillo/featurelayer"
	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"
	"github.com/bernardoforcillo/featurelayer/flags"
)

// REPLACE THESE. Everything in this file is the template's starter catalog: it
// shows each building block once so you can copy the shape, then delete or
// rename the entries for your product. Declare a feature in the same commit
// that adds the check that uses it; an entry nobody checks is a gate that
// exists on paper only.

// Feature keys. A key is a stable identifier: renaming one orphans stored
// usage counters and any per-tenant grant that names it, so add, don't rename.
const (
	// DataExport is a boolean feature: on or off per plan, no meter.
	DataExport catalog.Key = "data.export"
	// APICalls is a metered feature with a monthly limit. It also carries the
	// kill-switch flag below, so one Require/Consume answers both "is it
	// switched on" and "is there budget left".
	APICalls catalog.Key = "api.calls"
)

// Plans.
const (
	PlanFree entitlement.PlanID = "free"
	PlanPro  entitlement.PlanID = "pro"
)

// AddOnExtraAPICalls tops up the monthly API-call budget. Add-on limits are
// added to the plan's, and it only applies to a tenant on Pro.
const AddOnExtraAPICalls entitlement.AddOnID = "extra-api-calls"

// Monthly API-call allowances.
const (
	FreeAPICalls  int64 = 1_000
	ProAPICalls   int64 = 100_000
	ExtraAPICalls int64 = 50_000
)

// Config is the complete set of definitions the engine evaluates. Definitions
// are code on purpose: featurelayer's Config also unmarshals from JSON, so
// moving them to a file or a table later changes the source, not the shape.
func Config() featurelayer.Config {
	return featurelayer.Config{
		Features: []catalog.Feature{
			{Key: DataExport, Name: "Data export", Lifecycle: catalog.GA,
				Description: "Download the workspace's data. Pro only."},
			{Key: APICalls, Name: "API calls", Lifecycle: catalog.GA,
				Description: "Metered monthly API budget."},
		},
		// Kill switch: Enabled:true with an On default is a no-op. Flip
		// Enabled to false and every APICalls check is refused with reason
		// flag_off, with no deploy. Rollouts and segment targeting hang off the
		// same Flag; see featurelayer/flags.
		Flags: []flags.Flag{
			{Feature: APICalls, Enabled: true, Default: flags.Serve{On: true}},
		},
		Plans: []entitlement.Plan{
			{
				ID: PlanFree, Name: "Free",
				Entitlements: []entitlement.Entitlement{
					entitlement.Limited(APICalls, FreeAPICalls, entitlement.Month),
				},
			},
			{
				// Extends: Pro inherits everything Free grants; a limit it
				// restates replaces Free's rather than adding to it.
				ID: PlanPro, Name: "Pro", Extends: PlanFree,
				Entitlements: []entitlement.Entitlement{
					{Feature: DataExport},
					entitlement.Limited(APICalls, ProAPICalls, entitlement.Month),
				},
			},
		},
		AddOns: []entitlement.AddOn{
			{
				ID: AddOnExtraAPICalls, Name: "Extra API calls",
				Requires: []entitlement.PlanID{PlanPro},
				Entitlements: []entitlement.Entitlement{
					entitlement.Limited(APICalls, ExtraAPICalls, entitlement.Month),
				},
			},
		},
	}
}
