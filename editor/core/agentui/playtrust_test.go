package agentui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlaytestTrustIsUserLocalAndScoped(t *testing.T) {
	config := t.TempDir()
	one, two := t.TempDir(), t.TempDir()
	native := NewPlaytestTrust(config, one, true)
	if native == nil || native.Allowed() {
		t.Fatal("new project must not be trusted")
	}
	if err := native.Allow(); err != nil {
		t.Fatal(err)
	}
	if !NewPlaytestTrust(config, one, true).Allowed() {
		t.Fatal("permission did not survive a new session")
	}
	if NewPlaytestTrust(config, two, true).Allowed() || NewPlaytestTrust(config, one, false).Allowed() {
		t.Fatal("permission escaped its project or playtest kind")
	}
	if _, err := os.Stat(filepath.Join(one, ".aego")); !os.IsNotExist(err) {
		t.Fatal("the project must not control or contain user trust")
	}
	if err := native.Revoke(); err != nil || native.Allowed() {
		t.Fatalf("revoke: %v", err)
	}
	if err := native.Revoke(); err != nil {
		t.Fatal(err)
	}
}

func TestPlaytestTrustFailsClosed(t *testing.T) {
	trust := NewPlaytestTrust(t.TempDir(), t.TempDir(), true)
	if err := trust.Allow(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trust.path(), []byte(`{"version":1,"project":"other","kind":"native"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if trust.Allowed() {
		t.Fatal("mismatched permission was accepted")
	}
	if err := os.Remove(trust.path()); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "permission.json")
	if err := os.WriteFile(target, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, trust.path()); err == nil && trust.Allowed() {
		t.Fatal("symlink permission was accepted")
	}
	if NewPlaytestTrust("", t.TempDir(), true).Allowed() {
		t.Fatal("missing configuration must not grant permission")
	}
}
