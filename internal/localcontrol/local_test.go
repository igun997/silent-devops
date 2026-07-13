package localcontrol_test

import (
	"context"
	"google.golang.org/grpc"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
	"silent-devops/internal/localcontrol"
	"testing"
)

func TestInterceptorAddsLocalAdminClaims(t *testing.T) {
	called := false
	interceptor := localcontrol.UnaryInterceptor()
	_, err := interceptor(localcontrol.ContextWithPeerUID(context.Background(), 0), nil, &grpc.UnaryServerInfo{FullMethod: devopsv1.FleetService_ListAgents_FullMethodName}, func(ctx context.Context, req any) (any, error) {
		claims, ok := auth.ClaimsFromContext(ctx)
		called = ok && claims.Role == devopsv1.Role_ROLE_ADMIN && claims.Subject == "local:uid:0"
		return nil, nil
	})
	if err != nil || !called {
		t.Fatalf("called=%v err=%v", called, err)
	}
}
func TestInterceptorRejectsUnknownPeer(t *testing.T) {
	_, err := localcontrol.UnaryInterceptor()(context.Background(), nil, &grpc.UnaryServerInfo{}, func(context.Context, any) (any, error) { return nil, nil })
	if err == nil {
		t.Fatal("unknown peer accepted")
	}
}
