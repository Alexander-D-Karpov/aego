package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryRules(t *testing.T) {
	cat, err := builtinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	violations, err := Check(Config{Root: filepath.Join("..", ".."), Catalog: cat})
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
		{"modules/*/editor", "modules/physics/editor", true},
		{"modules/*/editor/...", "modules/physics/editor/x", true},
		{"modules/*/editor", "modules/physics", false},
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
	r, _ = ruleFor("gpu/internal/glstate")
	if r.pkg != "gpu/internal/..." {
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
	for _, rt := range []string{"script/vm", "script/runtime", "modules/scripting/systems", "script/reload", "script/stdlib", "script/bind"} {
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
		{"modules/render2d", "modules/render3d"}, {"modules/render3d", "modules/render2d"},
		{"modules/render2d", "modules/scripting"}, {"modules/render3d", "modules/scripting"},
		{"modules/scripting", "modules/render2d"}, {"modules/scripting", "modules/render3d"},
		{"engine/module", "editor/panels/inspector"}, {"scene", "engine/module"}, {"scene/frame", "engine/module"},
		{"sdk", "app/assembly/builtin"}, {"editor/panels/inspector", "app/assembly/builtin"},
	}
	for _, p := range pairs {
		from, _ := groupOf(p[0])
		to, _ := groupOf(p[1])
		if groupRule(from, to) == "" && layerRule(p[0], p[1]) == "" {
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

func TestRuntimePackagesExcludeDev(t *testing.T) {
	pkgs, err := RuntimePackages(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, p := range pkgs {
		local := strings.TrimPrefix(p, "aego/")
		if allowed(runtimeForbidden, local) {
			t.Errorf("runtime list contains dev package %s", local)
		}
		have[local] = true
	}
	for _, want := range []string{"engine", "scene", "script/runtime", "sdk"} {
		if !have[want] {
			t.Errorf("runtime list misses %s", want)
		}
	}
	for _, dev := range []string{"app", "cmd/aego", "editor/core/context", "project", "script/compiler"} {
		if have[dev] {
			t.Errorf("runtime list contains %s", dev)
		}
	}
}

func TestRuleCases(t *testing.T) {
	cases := []struct {
		dir     string
		catalog *Catalog
		want    []string // "rule where" substrings; empty means clean
	}{
		{"clean", &Catalog{Modules: []CatalogModule{{ID: "foo", Dir: "modules/foo"}}}, nil},
		{"module-module", nil, []string{"modules/a/a.go: [module-imports-module] imports modules/b"}},
		{"kernel-module", nil, []string{"scene/x/x.go: [kernel-imports-module] imports modules/a"}},
		{"panel-panel", nil, []string{"editor/panels/p1/p1.go: [panel-imports-panel] imports editor/panels/p2"}},
		{"runtime-dev", nil, []string{"engine/y: [runtime-reaches-dev] reaches project via engine/y -> project"}},
		{"module-own-editor", nil, []string{"modules/a/a.go: [module-imports-own-editor] imports modules/a/editor"}},
		{"literal", &Catalog{Modules: []CatalogModule{{ID: "fakemod", Dir: "modules/fakemod"}}}, []string{
			`editor/panels/inspector/inspector.go:3: [module-literal] "fakemod.Thing" names module fakemod`,
			`scene/s/s.go:3: [module-literal] "FAK001" names module fakemod`,
		}},
		{"color-literal", &Catalog{}, []string{
			"editor/panels/p/p.go:5: [color-literal] writes a mathx.Color literal",
			"editor/panels/p/p.go:7: [color-literal] calls mathx.ColorRGB8",
			"editor/panels/p/p.go:9: [color-literal] writes a mathx.Color literal",
		}},
		{"empty-reason", nil, []string{"[exception-syntax]", "has no reason", "unknown variant \"tset\""}},
		{"unused-exception", nil, []string{"[exception-unused] core/a -> modules/x"}},
		{"file-exception", nil, []string{"sdk/other.go: [sdk-imports-editor] imports editor/core/c"}},
		{"wasm", nil, []string{"[wasm-reaches-forbidden] reaches modules/accessibility via cmd/aego-playground -> modules/accessibility"}},
		{"no-group", nil, []string{"weird/x: [no-group]"}},
		{"gameplay-modules", nil, []string{
			"modules/gameplay/g.go: [module-imports-module] imports modules/physics",
			"modules/gameplay/g.go: [module-imports-module] imports modules/camera",
			"modules/gameplay/g.go: [module-imports-module] imports modules/audio",
		}},
		{"audio-render", nil, []string{"modules/audio/a.go: [layer:modules/audio] imports render/view", "modules/audio/editor/e.go: [layer:modules/audio/editor] imports render/view"}},
		{"camera-render2d", nil, []string{"modules/camera/c.go: [module-imports-module] imports modules/render2d", "modules/camera/c.go: [layer:modules/camera] imports render/batch"}},
		{"vorbis-internal", nil, []string{"audio/codec/vorbis/internal/vorbis/v.go: [layer:audio/codec/vorbis/internal/vorbis] imports core/mathx"}},
	}
	for _, c := range cases {
		t.Run(c.dir, func(t *testing.T) {
			vs, err := Check(Config{Root: filepath.Join("testdata", c.dir), Catalog: c.catalog})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, v := range vs {
				got = append(got, v.String())
			}
			all := strings.Join(got, "\n")
			if len(c.want) == 0 && len(got) > 0 {
				t.Fatalf("clean tree has violations:\n%s", all)
			}
			for _, w := range c.want {
				if !strings.Contains(all, w) {
					t.Errorf("missing %q in:\n%s", w, all)
				}
			}
			if c.dir == "audio-render" && strings.Contains(all, "imports render/debug") {
				t.Errorf("render/debug is allowed in the audio editor:\n%s", all)
			}
			if c.dir == "color-literal" && (strings.Contains(all, "ui/theme") || strings.Contains(all, "p.go:11") || strings.Contains(all, "p.go:13")) {
				t.Errorf("the theme, a zero colour or a list of tokens is reported:\n%s", all)
			}
			if c.dir == "file-exception" && strings.Contains(all, "sdk/editor.go") {
				t.Errorf("the excepted file is reported:\n%s", all)
			}
		})
	}
}

func TestGoListErrorFails(t *testing.T) {
	root := t.TempDir()
	write := func(name, data string) {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module aego\n\ngo 1.27\n")
	write("scene/x/x.go", "package x\n\nimport _ \"aego/scene/missing\"\n")
	if _, err := Check(Config{Root: root}); err == nil || !strings.Contains(err.Error(), "go list reported errors") {
		t.Fatalf("err = %v", err)
	}
}
