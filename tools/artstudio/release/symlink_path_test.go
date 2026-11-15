package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Resolve only paths belonging to test-owned fixtures. On macOS t.TempDir can
// be under /var, which is itself a symlink to /private/var. Production path
// validation must still reject symlinks rather than resolving caller input.
func physicalFixturePath(t *testing.T, p string) string {
	t.Helper()
	physical, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("resolve fixture path %s: %v", p, err)
	}
	return physical
}

func physicalTempDir(t *testing.T) string {
	t.Helper()
	return physicalFixturePath(t, t.TempDir())
}

func fixtureSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatalf("create symlink %s: %v", link, err)
	}
}

func requireRefusedSymlink(t *testing.T, p, link string) {
	t.Helper()
	err := noSymlinkPath(p)
	want := "symlink path refused: " + link
	if err == nil || err.Error() != want {
		t.Fatalf("noSymlinkPath(%q) = %v; want %q", p, err, want)
	}
}

func TestNoSymlinkPathChecksEveryComponent(t *testing.T) {
	base := physicalTempDir(t)
	dir := filepath.Join(base, "real", "child")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := noSymlinkPath(file); err != nil {
		t.Fatalf("physical fixture refused: %v", err)
	}
	if err := noSymlinkPath(filepath.Join(base, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing path = %v; want not-exist error", err)
	}

	for _, tc := range []struct {
		name, target, link string
		suffix             []string
	}{
		{"leaf", dir, filepath.Join(base, "leaf"), nil},
		{"ancestor", filepath.Join(base, "real"), filepath.Join(base, "ancestor"), []string{"child", "file"}},
		{"relative ancestor", "child", filepath.Join(base, "real", "relative"), []string{"file"}},
		{"dangling leaf", filepath.Join(base, "missing"), filepath.Join(base, "dangling"), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixtureSymlink(t, tc.target, tc.link)
			p := filepath.Join(append([]string{tc.link}, tc.suffix...)...)
			requireRefusedSymlink(t, p, tc.link)
		})
	}
}

func TestPhysicalFixturePathResolvesOnlyTestOwnedAlias(t *testing.T) {
	base := physicalTempDir(t)
	physical := filepath.Join(base, "real", "temp")
	if err := os.MkdirAll(physical, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "temp-alias")
	fixtureSymlink(t, filepath.Join(base, "real"), alias)
	throughAlias := filepath.Join(alias, "temp")
	requireRefusedSymlink(t, throughAlias, alias)
	if got := physicalFixturePath(t, throughAlias); got != physical {
		t.Fatalf("physical fixture path = %q; want %q", got, physical)
	} else if err := noSymlinkPath(got); err != nil {
		t.Fatalf("resolved fixture path refused: %v", err)
	}

	link := filepath.Join(physical, "untrusted")
	fixtureSymlink(t, base, link)
	requireRefusedSymlink(t, link, link)
}
