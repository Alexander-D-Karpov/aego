package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryRules(t *testing.T) {
	violations, err := Check(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Error(v)
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, pkg string
		want         bool
	}{
		{"core/...", "core/mathx", true},
		{"core/...", "core", true},
		{"core/...", "coreutil", false},
		{"platform", "platform/sdl", false},
		{"platform/...", "platform/sdl", true},
		{"...", "anything/at/all", true},
	}
	for _, c := range cases {
		if got := match(c.pattern, c.pkg); got != c.want {
			t.Fatalf("match(%q, %q) = %v", c.pattern, c.pkg, got)
		}
	}
}

func TestRuleSpecificity(t *testing.T) {
	r, ok := ruleFor("platform/sdl")
	if !ok || r.pkg != "platform/sdl" {
		t.Fatalf("rule = %v ok=%v", r.pkg, ok)
	}
	r, _ = ruleFor("gpu/gl33/internal/glstate")
	if r.pkg != "gpu/gl33/internal/..." {
		t.Fatalf("internal rule = %v", r.pkg)
	}
	if _, ok := ruleFor("nowhere/at/all"); ok {
		t.Fatal("unknown package matched a rule")
	}
}

func TestStageSixGraph(t *testing.T) {
	if _, ok := ruleFor("script/build"); ok {
		t.Fatal("script/build must not have a rule, the package is removed")
	}
	for _, rt := range []string{"script/vm", "script/runtime", "script/module", "script/reload", "script/stdlib", "script/bind"} {
		for _, dev := range []string{"script/workspace", "script/langsvc", "script/harness"} {
			found := false
			for _, d := range denied {
				if match(d.pkg, rt) && allowed(d.deny, dev) {
					found = true
				}
			}
			if !found {
				t.Errorf("%s may import %s", rt, dev)
			}
		}
	}
}

func TestModuleGraph(t *testing.T) {
	pairs := [][2]string{
		{"render2d", "render3d"}, {"render3d", "render2d"},
		{"render2d", "script/engine"}, {"render3d", "script/engine"},
		{"script/engine", "render2d"}, {"script/engine", "render3d"},
		{"engine/module", "editor/inspector"}, {"scene", "engine/module"}, {"scene/frame", "engine/module"},
		{"sdk", "engine/builtin"}, {"editor/inspector", "engine/builtin"},
	}
	for _, p := range pairs {
		blocked := false
		for _, d := range denied {
			if match(d.pkg, p[0]) && allowed(d.deny, p[1]) {
				blocked = true
			}
		}
		r, _ := ruleFor(p[0])
		if !blocked && allowed(r.allow, p[1]) {
			t.Errorf("%s may import %s", p[0], p[1])
		}
	}
}

func TestNoLegacyPackages(t *testing.T) {
	root := filepath.Join("..", "..")
	needles := []string{`"aego/scene/components"`, `"aego/scene/systems"`, "importers.All", "stdlib.Register("}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); path != root && (strings.HasPrefix(n, ".") || n == "third_party" || n == "depcheck") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, n := range needles {
			if strings.Contains(string(data), n) {
				t.Errorf("%s still references %s", path, n)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
