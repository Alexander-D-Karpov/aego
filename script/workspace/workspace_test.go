package workspace

import (
	scriptstdlib "aego/modules/scripting/stdlib"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"aego/script/bind"
	"aego/script/bind/manifest"
	"aego/script/diag"
	"aego/script/stdlib"
)

const srcA = "package a\n\nfunc Add(x int, y int) int {\n\treturn x + y\n}\n"
const srcB = "package b\n\nimport \"game/a\"\n\nfunc Twice(x int) int {\n\treturn a.Add(x, x)\n}\n"

func write(t *testing.T, root, rel, src string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func open(t *testing.T) (*Workspace, string) {
	t.Helper()
	root := t.TempDir()
	write(t, root, "scripts/a/a.aes", srcA)
	write(t, root, "scripts/b/b.aes", srcB)
	m, err := stdlibManifest()
	if err != nil {
		t.Fatal(err)
	}
	tab, err := m.Table()
	if err != nil {
		t.Fatal(err)
	}
	w, err := Open(Config{Root: root, Module: "game", Bindings: tab, Docs: m})
	if err != nil {
		t.Fatal(err)
	}
	return w, root
}

func TestOverlayOnlyInAnalysis(t *testing.T) {
	w, _ := open(t)
	if n := w.Snapshot().Errors(); n != 0 {
		t.Fatalf("clean project has %d errors", n)
	}
	w.SetOverlay("scripts/a/a.aes", []byte("package a\n\nfunc Add(x int) int {\n\treturn y\n}\n"))
	if w.Snapshot().Errors() == 0 {
		t.Fatal("overlay errors are not visible to analysis")
	}
	g, ds := w.Build()
	if diag.HasErrors(ds) || len(g.Failed) != 0 {
		t.Fatal("build saw an unsaved overlay")
	}
	w.ClearOverlay("scripts/a/a.aes")
	if w.Snapshot().Errors() != 0 {
		t.Fatal("cleared overlay still affects analysis")
	}
}

func TestIncremental(t *testing.T) {
	w, root := open(t)
	g1, _ := w.Build()
	if g1.ID != 1 || !slices.Equal(g1.Changed, []string{"game/a", "game/b"}) {
		t.Fatalf("first build: id %d changed %v", g1.ID, g1.Changed)
	}
	write(t, root, "scripts/a/a.aes", strings.Replace(srcA, "x + y", "y + x", 1))
	w.Changed([]string{"scripts/a/a.aes"})
	g2, _ := w.Build()
	if g2.ID != 2 || !slices.Equal(g2.Changed, []string{"game/a"}) {
		t.Fatalf("body edit: id %d changed %v", g2.ID, g2.Changed)
	}
	if g2.Modules["game/b"] != g1.Modules["game/b"] {
		t.Fatal("a body edit recompiled a dependent")
	}
	write(t, root, "scripts/a/a.aes", srcA+"\nfunc Sub(x int, y int) int {\n\treturn x - y\n}\n")
	w.Changed([]string{filepath.Join(root, "scripts", "a", "a.aes")})
	g3, _ := w.Build()
	if !slices.Equal(g3.Changed, []string{"game/a", "game/b"}) {
		t.Fatalf("interface edit changed %v", g3.Changed)
	}
	g4, _ := w.Build()
	if g4.ID != g3.ID || len(g4.Changed) != 0 {
		t.Fatal("an empty build bumped the generation")
	}
}

func TestFailedKeepsModules(t *testing.T) {
	w, root := open(t)
	g1, _ := w.Build()
	write(t, root, "scripts/a/a.aes", "package a\n\nfunc Add(x int, y int) int {\n\treturn z\n}\n")
	w.Changed([]string{"scripts/a/a.aes"})
	g2, ds := w.Build()
	if !diag.HasErrors(ds) || len(g2.Failed["game/a"]) == 0 {
		t.Fatal("broken package is not reported")
	}
	if g2.ID != g1.ID || g2.Modules["game/a"] != g1.Modules["game/a"] {
		t.Fatal("a failed build replaced the committed module")
	}
}

func TestBuildWithTests(t *testing.T) {
	w, root := open(t)
	write(t, root, "scripts/a/a_test.aes", "package a\n\nfunc TestAdd() {\n}\n")
	w.Changed([]string{"scripts/a/a_test.aes"})
	g1, _ := w.Build()
	if g1.Modules["game/a"].FuncIndex("TestAdd") >= 0 {
		t.Fatal("committed build compiled a test file")
	}
	tg, ds := w.BuildWith(BuildOptions{IncludeTests: true})
	if diag.HasErrors(ds) || tg.Modules["game/a"].FuncIndex("TestAdd") < 0 {
		t.Fatal("test build lost the test function")
	}
	if w.Committed() != g1 {
		t.Fatal("test build replaced the committed generation")
	}
	if !IsTest("scripts/a/a_test.aes") || IsTest("scripts/a/a.aes") {
		t.Fatal("IsTest")
	}
}

func stdlibManifest() (*manifest.Manifest, error) {
	r := bind.NewManifest()
	scriptstdlib.Install(r, stdlib.Options{Seed: 1, Testing: true})
	return manifest.FromRegistry(r, nil)
}

func TestHoldBack(t *testing.T) {
	w, root := open(t)
	g1, _ := w.Build()
	write(t, root, "scripts/a/a.aes", srcA+"\nfunc Sub(x int, y int) int {\n\treturn x - y\n}\n")
	write(t, root, "scripts/b/b.aes", "package b\n\nimport \"game/a\"\n\nfunc Twice(x int) int {\n\treturn a.Add(x, z)\n}\n")
	w.Changed([]string{"scripts/a/a.aes", "scripts/b/b.aes"})
	g2, _ := w.Build()
	if g2.ID != g1.ID || g2.Modules["game/a"] != g1.Modules["game/a"] {
		t.Fatal("a package with a new interface was committed while its broken dependent keeps the old module")
	}
	fa := g2.Failed["game/a"]
	if len(fa) == 0 || fa[0].Code != diag.CompileBlocked {
		t.Fatalf("game/a failures %v", fa)
	}
	if len(g2.Failed["game/b"]) == 0 {
		t.Fatal("game/b errors are not reported")
	}
}
