package docs

import (
	"testing"

	"aego/app/docicons"
	regdocs "aego/engine/docs"
	"aego/engine/res"
	"aego/gpu/null"
	"aego/render/icons"
	"aego/render/texture"
	"aego/ui/core"
	"aego/vfs"
)

func embeddedIcons(t *testing.T) *icons.Set {
	t.Helper()
	v, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	set, err := icons.Load(v, texture.NewUploader(null.New()), "engine://icons")
	if err != nil {
		t.Fatalf("load the baked icon atlas: %v", err)
	}
	return set
}

func TestEntryIconSemantics(t *testing.T) {
	p := New(testRegistry(), nil, nil)
	for _, tc := range []struct {
		name string
		e    regdocs.Entry
		want string
	}{
		{"task overrides metadata", regdocs.Entry{ID: "quickstart", Kind: regdocs.Page, Icon: "doc.tutorial"}, "doc.project"},
		{"metadata overrides reference", regdocs.Entry{ID: "aego/audio", Kind: regdocs.Module, Icon: "file.audio"}, "file.audio"},
		{"script package", regdocs.Entry{ID: "aego/rand", Kind: regdocs.Module}, "shape.dice"},
		{"project section", regdocs.Entry{ID: "section/animation", Kind: regdocs.Page}, "animation.player"},
		{"type inherits script package", regdocs.Entry{ID: "aego/audio.Sound", Parent: "aego/audio", Kind: regdocs.Type}, "cat.audio"},
		{"other script package", regdocs.Entry{ID: "aego/custom", Kind: regdocs.Module}, "code.block"},
		{"page inherits task", regdocs.Entry{ID: "quickstart/child", Parent: "quickstart", Kind: regdocs.Page}, "doc.project"},
		{"page inherits explicit icon", regdocs.Entry{ID: "flight/child", Parent: "flight", Kind: regdocs.Page}, "file.prefab"},
		{"unknown parent", regdocs.Entry{ID: "none/child", Parent: "none", Kind: regdocs.Page}, "help.book"},
		{"field", regdocs.Entry{ID: "speed", Kind: regdocs.Field, Title: "Speed", Field: &regdocs.FieldInfo{Type: "float"}}, "vec.velocity"},
		{"field without metadata", regdocs.Entry{ID: "missing", Kind: regdocs.Field}, "code.hints"},
		{"function", regdocs.Entry{Kind: regdocs.Function}, "code.brackets"},
		{"diagnostic", regdocs.Entry{Kind: regdocs.Code}, "log.error"},
		{"unknown kind", regdocs.Entry{Kind: regdocs.Kind(255)}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.entryIcon(tc.e); got != tc.want {
				t.Errorf("entryIcon(%+v) = %q, want %q", tc.e, got, tc.want)
			}
		})
	}
}

func TestInjectedNavigationPolicyCoversEveryBuiltinMapping(t *testing.T) {
	p := New(testRegistry(), nil, nil)
	tasks, refs := docicons.Builtin()
	for id, want := range tasks {
		// Task overrides take precedence even over explicitly supplied metadata.
		if got := p.entryIcon(regdocs.Entry{ID: id, Kind: regdocs.Page, Icon: "doc.tutorial"}); got != want {
			t.Errorf("task %q = %q, want %q", id, got, want)
		}
	}
	for id, want := range refs {
		if got := p.entryIcon(regdocs.Entry{ID: id, Kind: regdocs.Module}); got != want {
			t.Errorf("reference %q = %q, want %q", id, got, want)
		}
		if got := p.entryIcon(regdocs.Entry{ID: id + ".Example", Parent: id, Kind: regdocs.Type}); got != want {
			t.Errorf("reference child of %q = %q, want %q", id, got, want)
		}
	}
	// Without a composition policy, or without a registry, the generic policy
	// still preserves metadata and gives unknown entries a safe kind icon.
	empty := New(regdocs.Assemble(regdocs.Sources{}), nil, nil)
	if got := empty.entryIcon(regdocs.Entry{ID: "unknown", Kind: regdocs.Page}); got != "help.book" {
		t.Errorf("empty policy fallback = %q", got)
	}
	if got := New(nil, nil, nil).entryIcon(regdocs.Entry{Kind: regdocs.Page, Icon: "file.prefab"}); got != "file.prefab" {
		t.Errorf("nil registry metadata icon = %q", got)
	}
}

func TestFieldIcons(t *testing.T) {
	for _, tc := range []struct{ name, typ, want string }{
		{"gravity", "", "vec.gravity"}, {"angular", "", "vec.angular"},
		{"velocity", "", "vec.velocity"}, {"impulse", "", "vec.force"},
		{"heading", "quat", "tool.rotate"}, {"size", "", "tool.scale"},
		{"offset", "", "tool.move"}, {"sprite", "", "file.texture"},
		{"shape", "mesh", "file.mesh"}, {"asset", "prefab", "file.prefab"},
		{"target", "scene", "file.scene"}, {"target", "NodeRef", "node.empty"},
		{"enabled", "bool", "ui.check"}, {"mask", "", "view.overlays"},
		{"fixed", "", "vec.fixed"}, {"other", "int", "code.hints"},
	} {
		if got := fieldIcon(tc.name, tc.typ); got != tc.want {
			t.Errorf("fieldIcon(%q, %q) = %q, want %q", tc.name, tc.typ, got, tc.want)
		}
	}
}

func TestNavigationIconsResolveInBakedAtlas(t *testing.T) {
	set := embeddedIcons(t)
	ids := map[string]bool{}
	tasks, refs := docicons.Builtin()
	for _, mappings := range []map[string]string{tasks, refs} {
		for _, id := range mappings {
			ids[id] = true
		}
	}
	for _, id := range kindIcons {
		ids[id] = true
	}
	for _, g := range regdocs.Groups() {
		ids[g.Icon] = true
	}
	for _, tc := range []struct{ name, typ string }{
		{"gravity", ""}, {"angular", ""}, {"velocity", ""}, {"force", ""}, {"rotation", ""},
		{"scale", ""}, {"position", ""}, {"sprite", ""}, {"", "mesh"}, {"", "prefab"},
		{"", "scene"}, {"", "NodeRef"}, {"", "bool"}, {"mask", ""}, {"fixed", ""}, {"", ""},
	} {
		ids[fieldIcon(tc.name, tc.typ)] = true
	}
	for id := range ids {
		got := availableIcon(id, set.Has)
		if id != "code.timer" && got != id {
			t.Errorf("built-in navigation icon %q missing from atlas (fallback %q)", id, got)
			continue
		}
		if id == "code.timer" && !set.Has(id) && got != "play.speed" {
			t.Errorf("timer fallback = %q", got)
		}
		for _, scale := range []float32{1, 2} {
			if _, ok := set.Region(got, 16, scale); !ok {
				t.Errorf("%q does not resolve at %gx", got, scale)
			}
		}
	}
}

func TestIconFallbackAndNarrowDock(t *testing.T) {
	set := embeddedIcons(t)
	ui := core.New(nil)
	ui.Icons = set
	for _, tc := range []struct {
		width float32
		depth int
		id    string
		want  string
	}{
		{200, 0, "file.download", "file.download"},
		{200, 0, "unknown.plugin", "help.book"},
		{200, 0, "", "help.book"},
		{30, 0, "file.download", ""},
		{100, 5, "file.download", ""},
	} {
		if got := treeIcon(ui, tc.width, tc.depth, tc.id); got != tc.want {
			t.Errorf("treeIcon(%g, %d, %q) = %q, want %q", tc.width, tc.depth, tc.id, got, tc.want)
		}
	}
	ui.Icons = nil
	if got := treeIcon(ui, 200, 0, "file.download"); got != "" {
		t.Errorf("missing atlas should leave the label unadorned, got %q", got)
	}
	if got := availableIcon("code.timer", func(id string) bool { return id == "code.timer" }); got != "code.timer" {
		t.Errorf("a future baked timer icon should be preferred, got %q", got)
	}
}
