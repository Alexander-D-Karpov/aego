package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/engine/i18n"
	"aego/project"
)

func TestSourceRecoveryCLIRequiresConfirmation(t *testing.T) {
	bundle := i18n.Default()
	previous := bundle.Language()
	bundle.SetLanguage(i18n.English)
	t.Cleanup(func() { bundle.SetLanguage(previous) })
	p, err := project.Create(t.TempDir(), "Recovery", false)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(p.AssetsDir(), "a.txt")
	if err := os.WriteFile(source, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(p.Root, ".aego", "source-transactions", "active")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	old, current := []byte("old"), []byte("new")
	if err := os.WriteFile(filepath.Join(dir, "before-0000"), old, 0o600); err != nil {
		t.Fatal(err)
	}
	version := func(data []byte) map[string]any {
		sum := sha256.Sum256(data)
		return map[string]any{"exists": true, "sha256": hex.EncodeToString(sum[:])}
	}
	header := map[string]any{"kind": "prepared", "version": 1, "id": "11111111111111111111111111111111", "entries": []any{
		map[string]any{"path": "assets/a.txt", "before": version(old), "after": version(current), "backup": "before-0000"},
	}}
	data, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "journal.jsonl"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runSourceRecovery([]string{"status", p.Root}, &out); err != nil || !strings.Contains(out.String(), "prepared") || !strings.Contains(out.String(), "assets/a.txt") {
		t.Fatalf("status: %s, %v", out.String(), err)
	}
	if err := runSourceRecovery([]string{"restore", p.Root}, &out); err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("restore without confirmation was accepted: %v", err)
	}
	if got, _ := os.ReadFile(source); !bytes.Equal(got, current) {
		t.Fatalf("status or unconfirmed restore changed source: %q", got)
	}
	out.Reset()
	if err := runSourceRecovery([]string{"restore", p.Root, "--confirm"}, &out); err != nil {
		t.Fatalf("confirmed restore: %v", err)
	}
	if got, _ := os.ReadFile(source); !bytes.Equal(got, old) {
		t.Fatalf("confirmed recovery did not restore source: %q", got)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatalf("resolved journal was not cleaned: %v", err)
	}
}

func TestSourceRecoveryCLIRejectsInvalidCommands(t *testing.T) {
	for _, args := range [][]string{nil, {"discard"}, {"status", "--confirm"}, {"restore", "--unknown"}} {
		if err := runSourceRecovery(args, io.Discard); err == nil {
			t.Fatalf("accepted invalid arguments: %v", args)
		}
	}
}
