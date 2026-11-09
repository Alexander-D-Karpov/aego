//go:build linux

package pipeline

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/core/log"
)

// Both failures use actual directory permissions in this test-owned project:
// the second publication fails, and rollback of the first publication fails.
// Neither error may be lost, and recovery stays an explicit decision.
func TestSourceWriteAndRollbackFailureRequireExplicitRecovery(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a privileged process can bypass the filesystem permission fixture")
	}
	p := newProject(t)
	a := write(t, p, "assets/first/a.txt", "old-a")
	b := write(t, p, "assets/second/b.txt", "old-b")
	pl := recoveryTestPipeline(t, p)
	defer pl.Close()
	firstDir, secondDir := filepath.Dir(a), filepath.Dir(b)
	defer func() {
		_ = os.Chmod(firstDir, 0755)
		_ = os.Chmod(secondDir, 0755)
	}()
	metaA, err := os.ReadFile(a + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	metaB, err := os.ReadFile(b + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	reached := false
	pl.sourceTransactionCheckpoint = func(stage, rel string) {
		if stage != "published" || rel != "assets/first/a.txt" {
			return
		}
		reached = true
		if err := os.Chmod(firstDir, 0555); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(secondDir, 0555); err != nil {
			t.Fatal(err)
		}
	}
	edits := []SourceEdit{
		{Path: "assets/first/a.txt", Before: []byte("old-a"), After: []byte("new-a")},
		{Path: "assets/second/b.txt", Before: []byte("old-b"), After: []byte("new-b")},
	}
	_, err = pl.ApplySources(edits, false)
	pl.sourceTransactionCheckpoint = nil
	if !reached || err == nil || !errors.Is(err, os.ErrPermission) || !strings.Contains(err.Error(), "assets/second/") || !strings.Contains(err.Error(), "cannot roll back assets/first/a.txt") {
		t.Fatalf("missing original write or rollback failure: reached=%v err=%v", reached, err)
	}
	t.Logf("real write and rollback failures preserved: %v", err)
	if got, err := os.ReadFile(a); err != nil || string(got) != "new-a" {
		t.Fatalf("expected the first source to remain interrupted: %q, %v", got, err)
	}
	if got, err := os.ReadFile(b); err != nil || string(got) != "old-b" {
		t.Fatalf("failed publication changed the second source: %q, %v", got, err)
	}
	if status, err := SourceRecoveryStatus(p); err != nil || status == nil || status.State != "prepared" {
		t.Fatalf("double failure did not retain the journal: %+v, %v", status, err)
	}
	if _, err := pl.ApplySources(edits, false); err == nil {
		t.Fatal("a subsequent edit overtook the pending recovery")
	}
	if other, err := Open(p, NewImporterRegistry(), log.Nop()); err == nil {
		other.Close()
		t.Fatal("opening the project silently accepted pending recovery")
	}
	if err := os.Chmod(firstDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(secondDir, 0755); err != nil {
		t.Fatal(err)
	}
	pl.Close()
	if err := RestoreSourceRecovery(p); err != nil {
		t.Fatalf("explicit recovery after fixing permissions: %v", err)
	}
	for name, want := range map[string][]byte{a: []byte("old-a"), b: []byte("old-b"), a + MetaSuffix: metaA, b + MetaSuffix: metaB} {
		got, err := os.ReadFile(name)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("recovery did not restore %s: %q, %v", name, got, err)
		}
	}
	if status, err := SourceRecoveryStatus(p); err != nil || status != nil {
		t.Fatalf("recovery remains pending: %+v, %v", status, err)
	}
}
