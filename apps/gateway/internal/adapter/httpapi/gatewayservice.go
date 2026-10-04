package httpapi

import (
	"context"

	"connectrpc.com/connect"

	gatewayv1 "github.com/bernardoforcillo/ignition/go-packages/proto/gen/gateway/v1"
)

// gatewayService implements gatewayv1connect.GatewayServiceHandler. It
// is a transport-layer handler, exactly like healthzHandler and
// readyzHandler beside it: it owns no business decision, and exists
// only to prove the Connect transport is wired correctly end-to-end
// (see /proto/gateway/v1/gateway.proto). It intentionally does not
// touch internal/core — there is no routing decision to make for a
// liveness-style ping.
type gatewayService struct{}

func newGatewayService() *gatewayService { return &gatewayService{} }

// Ping implements gatewayv1connect.GatewayServiceHandler.
func (s *gatewayService) Ping(
	_ context.Context,
	req *connect.Request[gatewayv1.PingRequest],
) (*connect.Response[gatewayv1.PingResponse], error) {
	return connect.NewResponse(&gatewayv1.PingResponse{
		Message: req.Msg.GetMessage(),
	}), nil
}
