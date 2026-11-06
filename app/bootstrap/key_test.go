package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aego/project"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type fixture struct {
	engine string
	game   *project.Project
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	engine := filepath.Join(root, "engine")
	writeFile(t, filepath.Join(engine, "go.mod"), "module aego\n\ngo 1.27\n")
	writeFile(t, filepath.Join(engine, "app", "run", "run.go"), "package run\n")
	writeFile(t, filepath.Join(engine, "engine", "res", "shaders", "sprite.vert"), "void main() {}\n")

	gameDir := filepath.Join(engine, "examples", "game")
	p, err := project.Create(gameDir, "Game", true)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(gameDir, "go.mod"),
		"module game\n\ngo 1.27\n\nrequire aego v0.0.0\n\nreplace aego => ../..\n")
	writeFile(t, filepath.Join(p.NativeDir(), "module.go"), "package native\n")
	return fixture{engine: engine, game: p}
}

func key(t *testing.T, p *project.Project) Key {
	t.Helper()
	k, err := CacheKey(p, HostTarget(), "test")
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestKeyTracksReplacedEngineSources(t *testing.T) {
	f := newFixture(t)
	base := key(t, f.game)
	if key(t, f.game) != base {
		t.Fatal("key is not deterministic")
	}
	writeFile(t, filepath.Join(f.engine, "app", "run", "run.go"), "package run\n\nconst X = 1\n")
	if key(t, f.game) == base {
		t.Fatal("editing the replaced engine did not change the key")
	}
}

func TestKeyTracksEmbeddedEngineAssets(t *testing.T) {
	f := newFixture(t)
	base := key(t, f.game)
	writeFile(t, filepath.Join(f.engine, "engine", "res", "shaders", "sprite.vert"), "void main() { }\n")
	if key(t, f.game) == base {
		t.Fatal("editing an embedded engine resource did not change the key")
	}
}

func TestKeyIgnoresIrrelevantFiles(t *testing.T) {
	f := newFixture(t)
	base := key(t, f.game)
	writeFile(t, filepath.Join(f.engine, "app", "run", "run_test.go"), "package run\n")
	writeFile(t, filepath.Join(f.engine, ".cache", "native", "libSDL3.a"), "binary")
	writeFile(t, filepath.Join(f.engine, ".git", "HEAD"), "ref")
	writeFile(t, filepath.Join(f.engine, "tools", "genfont", "go.mod"), "module genfont\n")
	writeFile(t, filepath.Join(f.engine, "tools", "genfont", "main.go"), "package main\n")
	writeFile(t, filepath.Join(f.game.BuildDir(), "abc", "game"), "binary")
	if key(t, f.game) != base {
		t.Fatal("tests, caches, nested modules or build outputs changed the key")
	}
}

func TestKeyTracksNativeSources(t *testing.T) {
	f := newFixture(t)
	base := key(t, f.game)
	writeFile(t, filepath.Join(f.game.NativeDir(), "module.go"), "package native\n\nconst Speed = 2\n")
	if key(t, f.game) == base {
		t.Fatal("editing native code did not change the key")
	}
}

func TestLocalReplaces(t *testing.T) {
	root := t.TempDir()
	absolute := filepath.VolumeName(root) + "/abs/c"
	writeFile(t, filepath.Join(root, "go.mod"), fmt.Sprintf(`module game

go 1.27

replace aego => ../engine // local engine

replace (
    example.com/a v1.2.3 => ./vendor/a
    example.com/b => example.com/b v1.0.0
    example.com/c => %s
)
`, absolute))
	got, err := LocalReplaces(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		filepath.Clean(filepath.Join(root, "../engine")):   true,
		filepath.Clean(filepath.Join(root, "vendor", "a")): true,
		filepath.Clean(absolute):                           true,
	}
	if len(got) != len(want) {
		t.Fatalf("replaces = %v", got)
	}
	for _, g := range got {
		if !want[g] {
			t.Fatalf("unexpected replace %q in %v", g, got)
		}
	}
}

func TestLocalReplacesWithoutGoMod(t *testing.T) {
	got, err := LocalReplaces(t.TempDir())
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestPruneKeepsRecentBuilds(t *testing.T) {
	dir := t.TempDir()
	names := []string{"a", "b", "c", "d", "e"}
	for i, n := range names {
		p := filepath.Join(dir, n)
		os.MkdirAll(p, 0o755)
		ts := int64(1_700_000_000 + i)
		os.Chtimes(p, unixTime(ts), unixTime(ts))
	}
	pruneBuilds(dir, "e", 3)
	for _, n := range []string{"e", "d", "c"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Fatalf("%s was pruned", n)
		}
	}
	for _, n := range []string{"a", "b"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			t.Fatalf("%s survived pruning", n)
		}
	}
}

func unixTime(s int64) time.Time {
	return time.Unix(s, 0)
}
