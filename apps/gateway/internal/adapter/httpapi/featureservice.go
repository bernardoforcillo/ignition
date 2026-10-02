package httpapi

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	featurelayer "github.com/bernardoforcillo/featurelayer"
	"github.com/bernardoforcillo/featurelayer/catalog"

	saasv1 "github.com/bernardoforcillo/ignition/go-packages/proto/gen/saas/v1"
)

// featureHandler implements saasv1connect.FeatureServiceHandler.
type featureHandler struct {
	features featureChecker
	members  workspaceMembership
}

// CheckFeature reports the decision for the caller's workspace without
// spending anything. A refusal is a normal answer (enabled=false with a
// reason), not an RPC error; only a store failure is an error.
func (h *featureHandler) CheckFeature(ctx context.Context, req *connect.Request[saasv1.CheckFeatureRequest]) (*connect.Response[saasv1.CheckFeatureResponse], error) {
	user, err := subjectOrErr(ctx)
	if err != nil {
		return nil, err
	}
	wsID, key := req.Msg.GetWorkspaceId(), req.Msg.GetFeatureKey()
	if wsID == "" {
		return nil, requiredField("workspace_id")
	}
	if key == "" {
		return nil, requiredField("feature_key")
	}
	// Only members may look at a workspace's entitlements.
	if _, err := h.members.Get(ctx, user, wsID); err != nil {
		return nil, toConnectError(ctx, err)
	}

	d := h.features.Evaluate(ctx, catalog.Key(key), wsID, user)
	if d.Reason == featurelayer.ReasonStoreError {
		return nil, toConnectError(ctx, fmt.Errorf("evaluating feature %q: %w", key, d.Err))
	}
	if d.Enabled && d.Entitlement != nil && d.Entitlement.Limit != nil {
		if d, err = h.features.Usage(ctx, catalog.Key(key), wsID, user); err != nil {
			return nil, toConnectError(ctx, fmt.Errorf("reading usage of feature %q: %w", key, err))
		}
	}

	resp := &saasv1.CheckFeatureResponse{Enabled: d.Enabled, Reason: string(d.Reason)}
	if d.Usage != nil && d.Usage.Max >= 0 {
		resp.Limit = &d.Usage.Max
		resp.Remaining = &d.Usage.Remaining
	}
	return connect.NewResponse(resp), nil
}
