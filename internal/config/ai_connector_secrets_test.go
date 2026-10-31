package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func newAccountStoreForTest(t *testing.T) *AccountStore {
	t.Helper()
	db, err := storage.New(filepath.Join(t.TempDir(), "gofer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewAccountStore(db, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestAIConnectorSecretsAreBoundToUserAndConnector(t *testing.T) {
	store := newAccountStoreForTest(t)
	encrypted, err := store.EncryptAIConnectorSecret("alice", "connector-1", "super-secret")
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == "" || strings.Contains(encrypted, "super-secret") {
		t.Fatal("secret was not sealed")
	}
	plaintext, err := store.DecryptAIConnectorSecret("alice", "connector-1", encrypted)
	if err != nil || plaintext != "super-secret" {
		t.Fatalf("decrypt = %q, %v", plaintext, err)
	}
	if _, err := store.DecryptAIConnectorSecret("bob", "connector-1", encrypted); err == nil {
		t.Fatal("secret decrypted for another user")
	}
	if _, err := store.DecryptAIConnectorSecret("alice", "connector-2", encrypted); err == nil {
		t.Fatal("secret decrypted for another connector")
	}
	empty, err := store.EncryptAIConnectorSecret("alice", "connector-1", "  ")
	if err != nil || empty != "" {
		t.Fatalf("empty secret = %q, %v", empty, err)
	}
}
