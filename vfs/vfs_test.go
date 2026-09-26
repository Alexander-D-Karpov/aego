package vfs

import (
	"os"
	"path/filepath"
	"testing"

	"aego/core/errs"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newVFS(t *testing.T, mounts ...Mount) *VFS {
	t.Helper()
	v, err := New(mounts...)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSchemes(t *testing.T) {
	res, user := t.TempDir(), t.TempDir()
	write(t, res, "shaders/basic.vert", "source")
	v := newVFS(t,
		Mount{Scheme: "res", Root: res, ReadOnly: true},
		Mount{Scheme: "user", Root: user},
	)
	data, err := v.ReadFile("res://shaders/basic.vert")
	if err != nil || string(data) != "source" {
		t.Fatalf("read res: %v %q", err, data)
	}
	if !v.Exists("res://shaders/basic.vert") || v.Exists("res://shaders/missing.vert") {
		t.Fatal("exists wrong")
	}
	if err := v.WriteFile("user://saves/slot1.bin", []byte{1, 2, 3}); err != nil {
		t.Fatalf("write user: %v", err)
	}
	got, err := v.ReadFile("user://saves/slot1.bin")
	if err != nil || len(got) != 3 {
		t.Fatalf("read user: %v %v", err, got)
	}
}

func TestReadOnlyAndErrors(t *testing.T) {
	res := t.TempDir()
	v := newVFS(t, Mount{Scheme: "res", Root: res, ReadOnly: true})
	if err := v.WriteFile("res://x.txt", nil); !errs.Has(err, errs.VFSReadOnly) {
		t.Fatalf("readonly: %v", err)
	}
	if _, err := v.ReadFile("res://missing"); !errs.Has(err, errs.VFSNotFound) {
		t.Fatalf("not found: %v", err)
	}
	if _, err := v.ReadFile("nope://x"); !errs.Has(err, errs.VFSUnknownScheme) {
		t.Fatalf("scheme: %v", err)
	}
	for _, bad := range []string{"res:/x", "x.txt", "res://", "res://a\\b"} {
		if _, err := v.ReadFile(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestEscapeIsClamped(t *testing.T) {
	_, rel, err := Split("res://../../etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "etc/passwd" {
		t.Fatalf("rel = %q", rel)
	}
	_, rel, _ = Split("res://a/./b/../c.txt")
	if rel != "a/c.txt" {
		t.Fatalf("rel = %q", rel)
	}
}

func TestOverlayOrder(t *testing.T) {
	base, over := t.TempDir(), t.TempDir()
	write(t, base, "a.txt", "base")
	write(t, base, "b.txt", "base-b")
	write(t, over, "a.txt", "over")
	v := newVFS(t,
		Mount{Scheme: "res", Root: base, ReadOnly: true},
		Mount{Scheme: "res", Root: over, ReadOnly: true},
	)
	if a, _ := v.ReadFile("res://a.txt"); string(a) != "over" {
		t.Fatalf("a = %q", a)
	}
	if b, _ := v.ReadFile("res://b.txt"); string(b) != "base-b" {
		t.Fatalf("b = %q", b)
	}
}

func TestExclusiveSchemeRejectsSecondMount(t *testing.T) {
	v := newVFS(t, Mount{Scheme: "user", Root: t.TempDir()})
	if err := v.Mount(Mount{Scheme: "user", Root: t.TempDir()}); !errs.Has(err, errs.VFSMount) {
		t.Fatalf("second user mount: %v", err)
	}
	if err := v.Mount(Mount{Scheme: "engine", Root: t.TempDir()}); err != nil {
		t.Fatalf("first engine mount: %v", err)
	}
	if err := v.Mount(Mount{Scheme: "engine", Root: t.TempDir()}); err != nil {
		t.Fatalf("engine is an overlay scheme: %v", err)
	}
	if err := v.Mount(Mount{Root: t.TempDir()}); !errs.Has(err, errs.VFSMount) {
		t.Fatalf("empty scheme: %v", err)
	}
	if _, err := New(Mount{Scheme: "user"}, Mount{Scheme: "user"}); !errs.Has(err, errs.VFSMount) {
		t.Fatalf("New must surface the mount error: %v", err)
	}
}
