package langsvc

import (
	"path/filepath"
	"strconv"
	"testing"
)

func TestProjectImpl(t *testing.T) {
	root := filepath.Join(t.TempDir(), "native")
	at := func(p string, line int) string { return p + ":" + strconv.Itoa(line) }
	cases := []struct {
		name string
		loc  string
		ok   bool
		line int
	}{
		{"project file", at(filepath.Join(root, "combat.go"), 12), true, 12},
		{"nested package", at(filepath.Join(root, "ai", "brain.go"), 3), true, 3},
		{"engine binding", at(filepath.Join(t.TempDir(), "aego", "scene", "bind.go"), 40), false, 0},
		{"sibling with prefix", at(filepath.Join(root+"x", "a.go"), 1), false, 0},
		{"escapes root", at(filepath.Join(root, "..", "x.go"), 1), false, 0},
		{"no line", filepath.Join(root, "a.go"), false, 0},
		{"empty", "", false, 0},
	}
	for _, c := range cases {
		got, ok := projectImpl(c.loc, root)
		if ok != c.ok || (ok && got.Line != c.line) {
			t.Errorf("%s: got %+v %v, want ok=%v line=%d", c.name, got, ok, c.ok, c.line)
		}
	}
	if _, ok := projectImpl(at(filepath.Join(root, "a.go"), 1), ""); ok {
		t.Error("an empty native root must not resolve")
	}
}
