package clientapi_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/clientapi"
)

type authClient struct{ request *devopsv1.LoginRequest }

func (a *authClient) Login(_ context.Context, r *devopsv1.LoginRequest, _ ...grpc.CallOption) (*devopsv1.LoginResponse, error) {
	a.request = r
	return &devopsv1.LoginResponse{AccessToken: "secret", Role: devopsv1.Role_ROLE_ADMIN}, nil
}
func (a *authClient) RedeemClientInvitation(context.Context, *devopsv1.RedeemClientInvitationRequest, ...grpc.CallOption) (*devopsv1.LoginResponse, error) {
	return &devopsv1.LoginResponse{AccessToken: "secret"}, nil
}

type fleetClient struct{ token string }

func TestAdapterLoginStoresTokenWithoutReturningIt(t *testing.T) {
	auth := &authClient{}
	store := &memoryStore{}
	api := clientapi.NewForTest(auth, nil, store)
	result, err := api.Call(context.Background(), "login", []string{"alice", "password"})
	if err != nil {
		t.Fatal(err)
	}
	if auth.request.Username != "alice" || store.token != "secret" {
		t.Fatal("login not persisted")
	}
	if result.(map[string]any)["access_token"] != nil {
		t.Fatal("token returned")
	}
}

type memoryStore struct{ token string }

func (m *memoryStore) Save(v string) error   { m.token = v; return nil }
func (m *memoryStore) Load() (string, error) { return m.token, nil }
func (m *memoryStore) Clear() error          { m.token = ""; return nil }
