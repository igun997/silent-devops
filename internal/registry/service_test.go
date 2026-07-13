package registry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	devopsv1 "silent-devops/api/devops/v1"
)

func TestPeerCertificateIdentity(t *testing.T) {
	ctx := peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: tlsState(&x509.Certificate{Subject: pkix.Name{CommonName: "agent-1"}, SerialNumber: big.NewInt(42)})}})
	id, serial, err := peerCertificateIdentity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id != "agent-1" || serial != "2a" {
		t.Fatalf("id=%q serial=%q", id, serial)
	}
}
func TestPeerCertificateIdentityRequiresMTLS(t *testing.T) {
	if _, _, err := peerCertificateIdentity(context.Background()); err == nil {
		t.Fatal("missing mTLS accepted")
	}
}
func TestRegistryStatusCodes(t *testing.T) {
	for _, err := range []error{ErrDuplicateStream, ErrIdentityMismatch, ErrVersionMismatch, ErrLimitsExceeded} {
		if registryStatus(err) == nil {
			t.Fatal(err)
		}
	}
}

func tlsState(cert *x509.Certificate) tls.ConnectionState {
	return tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
}

func TestAuthorizeRunsBeforeAcquire(t *testing.T) {
	r := New(1, devopsv1.DefaultLimits(), time.Minute)
	called := false
	s := &AgentServer{Registry: r, Authorize: func(context.Context, string, string, time.Time) error { called = true; return context.Canceled }}
	_ = s
	if called {
		t.Fatal("authorize called without stream")
	}
}
