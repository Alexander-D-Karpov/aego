package export

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestVerifiedLaunchPinsContents(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "game")
	if err := os.WriteFile(bin, []byte("good"), 0700); err != nil {
		t.Fatal(err)
	}
	seal, err := Seal(dir)
	if err != nil {
		t.Fatal(err)
	}
	clone, err := VerifiedCopy(dir, seal)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(clone)
	if err := os.WriteFile(bin, []byte("evil"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifiedCopy(dir, seal); err == nil {
		t.Fatal("replaced binary accepted")
	}
	got, err := os.ReadFile(filepath.Join(clone, "game"))
	if err != nil || !bytes.Equal(got, []byte("good")) {
		t.Fatalf("private launch changed %s %v", got, err)
	}
	if err := os.WriteFile(bin, []byte("good"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "preload.dll"), []byte("extra"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifiedCopy(dir, seal); err == nil {
		t.Fatal("extra file in sealed export accepted")
	}
}
func TestExportSealRefusesChildLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged symlinks are not available by default on Windows")
	}
	dir := t.TempDir()
	secret := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(secret, []byte("confined"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, "game")); err != nil {
		t.Fatal(err)
	}
	if _, err := Seal(dir); err == nil {
		t.Fatal("symlink into another tree was sealed")
	}
}
