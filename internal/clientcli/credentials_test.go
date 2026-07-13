package clientcli_test

import (
	"os"
	"path/filepath"
	"testing"

	"silent-devops/internal/clientcli"
)

func TestCredentialStoreRootOnlyRoundTripAndClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	store := clientcli.CredentialStore{Path: path}
	if err := store.Save("access-token"); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	token, err := store.Load()
	if err != nil || token != "access-token" {
		t.Fatalf("token=%q err=%v", token, err)
	}
	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("credential remains")
	}
}
func TestGrammarRejectsMalformedCommands(t *testing.T) {
	bad := [][]string{{"agents"}, {"agents", "bad"}, {"services", "start"}, {"cleanup", "run"}, {"ssh"}, {"login", "only-user"}}
	for _, args := range bad {
		if clientcli.Validate(args) == nil {
			t.Errorf("accepted %v", args)
		}
	}
	good := [][]string{{"agents", "list"}, {"services", "status", "a", "sshd.service"}, {"cleanup", "preview", "a", "/tmp/x"}, {"login", "u", "p"}, {"tui"}}
	for _, args := range good {
		if err := clientcli.Validate(args); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
}
