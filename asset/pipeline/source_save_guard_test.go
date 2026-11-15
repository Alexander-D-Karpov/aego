package pipeline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuardSourceWriteRejectsPendingJournalWithoutRunningCallback(t *testing.T) {
	p := newProject(t)
	pl := openPipeline(t, p, &fakeImporter{version: 1})
	called := 0
	if err := pl.GuardSourceWrite(func() error { called++; return nil }); err != nil || called != 1 {
		t.Fatalf("ordinary source write was not allowed: calls=%d, err=%v", called, err)
	}
	journal := filepath.Join(p.Root, filepath.FromSlash(sourceJournalDir))
	if err := os.MkdirAll(journal, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(journal, "unresolved")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pl.GuardSourceWrite(func() error { called++; return nil }); err == nil || !strings.Contains(err.Error(), "source transaction") {
		t.Fatalf("pending journal did not block source write: %v", err)
	}
	if called != 1 {
		t.Fatal("the blocked source write callback ran")
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "keep" {
		t.Fatalf("the guard changed pending recovery: %q, %v", got, err)
	}
}

func TestCheckSourceRecoveryIsReadOnly(t *testing.T) {
	p := newProject(t)
	pl := openPipeline(t, p, &fakeImporter{version: 1})
	if err := pl.CheckSourceRecovery(); err != nil {
		t.Fatalf("unexpected source recovery: %v", err)
	}
	journal := filepath.Join(p.Root, filepath.FromSlash(sourceJournalDir))
	if err := os.MkdirAll(journal, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(journal, "unresolved")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pl.CheckSourceRecovery(); err == nil || !strings.Contains(err.Error(), "source transaction") {
		t.Fatalf("pending journal did not block the read-only check: %v", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "keep" {
		t.Fatalf("the check changed pending recovery: %q, %v", got, err)
	}
}

func TestGuardSourceWritePropagatesWriteErrorsAndReleasesPipeline(t *testing.T) {
	p := newProject(t)
	pl := openPipeline(t, p, &fakeImporter{version: 1})
	want := errors.New("write failure")
	if err := pl.GuardSourceWrite(func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("write error was not preserved: %v", err)
	}
	// This uses the normal pipeline lock, and would deadlock if a rejected
	// callback retained the guard.
	if _, _, err := pl.Scan(); err != nil {
		t.Fatalf("pipeline stayed locked after a failed save: %v", err)
	}
}
