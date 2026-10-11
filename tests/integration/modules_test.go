package integration

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/editor/inspector"
	"aego/editor/templates"
	"aego/engine/builtin"
	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/schema"
	"aego/script/bind"
	"aego/script/bind/manifest"
	"aego/script/stdlib"
	"aego/sdk"
)

var updateModules = flag.Bool("update-modules", false, "rewrite module golden files")

type probeRotator struct {
	Speed float32 `aego:"speed"`
}

type probeModule struct{}

func (probeModule) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "rotator"}
}

func (probeModule) Register(r *sdk.Registry) error {
	return sdk.Component[probeRotator](r, sdk.Name("game.Rotator")).Template("Rotator", nil).Err()
}

func moduleGolden(t *testing.T, name string, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateModules {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden missing, run with -update-modules: %v", err)
	}
	if string(want) != got {
		t.Fatalf("%s differs:\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}

func probeTypes(t *testing.T) (*scene.Types, []schema.TemplateDesc) {
	t.Helper()
	types, err := builtin.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	man, err := builtin.Load(types, bind.New(), stdlib.Options{Seed: 1}, sdk.Adapt(probeModule{}))
	if err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return types, man.Templates
}

func TestCreateMenuLayout(t *testing.T) {
	_, descs := probeTypes(t)
	reg := templates.New()
	if err := reg.Load(descs); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, sec := range templates.Layout(reg, templates.Place{}) {
		for _, it := range sec.Items {
			if sec.Title != "" {
				b.WriteString(sec.Title + "/")
			}
			b.WriteString(it.Name + "\n")
		}
	}
	moduleGolden(t, "create_menu.txt", b.String())
}

func TestInspectorSections(t *testing.T) {
	types, _ := probeTypes(t)
	sr := types.Schema()
	reg := inspector.NewRegistry()
	if err := reg.Load(sr); err != nil {
		t.Fatal(err)
	}
	var list []schema.TypeID
	for _, td := range sr.Components() {
		if td.FullName == "engine.Unknown" || td.ID == scriptcomp.ScriptTypeID {
			continue
		}
		list = append(list, td.ID)
	}
	var b strings.Builder
	for _, sec := range inspector.Sections(reg, sr, list) {
		for _, id := range sec.Types {
			td, _ := sr.ByID(id)
			m := reg.Meta(td)
			badge := m.Badge
			if badge == "" {
				badge = "-"
			}
			b.WriteString(strings.Join([]string{sec.Category, td.FullName, m.DisplayName, m.Icon, badge}, "\t") + "\n")
		}
	}
	moduleGolden(t, "inspector.txt", b.String())
}

func withoutLoc(data []byte) string {
	var b strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "loc=") {
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

func TestBindingsManifestUnchanged(t *testing.T) {
	m, err := builtin.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	moduleGolden(t, "bindings.aetext", withoutLoc(manifest.Encode(m)))
}
