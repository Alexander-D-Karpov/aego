package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"aego/app/docicons"
	"aego/app/docstool"
	regdocs "aego/engine/docs"
)

var editorIconLike = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z0-9_]+$`)

func editorIconCallName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}

func collectEditorIconUses(fset *token.FileSet, f *ast.File, out map[string]string) {
	lit := func(e ast.Expr) {
		bl, ok := e.(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			return
		}
		s, err := strconv.Unquote(bl.Value)
		if err != nil || !editorIconLike.MatchString(s) {
			return
		}
		if _, seen := out[s]; !seen {
			out[s] = fset.Position(bl.Pos()).String()
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			switch name := editorIconCallName(x.Fun); {
			case name == "IconAt" && len(x.Args) >= 3:
				lit(x.Args[2])
			case (name == "Button" || name == "TintButton") && len(x.Args) >= 4:
				lit(x.Args[1])
			case name == "Describe" && len(x.Args) >= 2:
				lit(x.Args[1])
			case name == "MenuItemIcon" && len(x.Args) >= 4:
				lit(x.Args[3])
			case name == "SubMenuIcon" && len(x.Args) >= 3:
				lit(x.Args[2])
			}
		case *ast.KeyValueExpr:
			if id, ok := x.Key.(*ast.Ident); ok && (id.Name == "Icon" || id.Name == "icon") {
				lit(x.Value)
			}
		case *ast.FuncDecl:
			if x.Body != nil && strings.Contains(strings.ToLower(x.Name.Name), "icon") {
				ast.Inspect(x.Body, func(m ast.Node) bool {
					if r, ok := m.(*ast.ReturnStmt); ok {
						for _, e := range r.Results {
							lit(e)
						}
					}
					return true
				})
			}
		}
		return true
	})
}

// Audit the same editor, UI and module-editor sources against the composed
// application registry, which includes module-owned icons as well as core icons.
func TestEditorIconsAreListed(t *testing.T) {
	root := filepath.Join("..", "..")
	dirs := []string{filepath.Join(root, "editor"), filepath.Join(root, "ui")}
	mods, err := filepath.Glob(filepath.Join(root, "modules", "*", "editor"))
	if err != nil {
		t.Fatal(err)
	}
	dirs = append(dirs, mods...)
	found := map[string]string{}
	fset := token.NewFileSet()
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			collectEditorIconUses(fset, f, found)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	listed := map[string]bool{}
	for _, u := range editorIcons() {
		listed[u.ID] = true
	}
	for id, at := range found {
		if !listed[id] {
			t.Errorf("%s uses icon %s, add it to the appropriate icon audit", at, id)
		}
	}
}

// The documentation navigation policies use map values rather than literal
// Icon fields or return statements. Audit those values too, without mistaking
// the documentation entry IDs used as map keys for icon IDs.
func docsIconMappings(fset *token.FileSet, f *ast.File, names ...string) (map[string]string, error) {
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = false
	}
	out := map[string]string{}
	for _, decl := range f.Decls {
		group, ok := decl.(*ast.GenDecl)
		if !ok || group.Tok != token.VAR {
			continue
		}
		for _, spec := range group.Specs {
			v := spec.(*ast.ValueSpec)
			for i, name := range v.Names {
				seen, matches := wanted[name.Name]
				if !matches {
					continue
				}
				if seen || i >= len(v.Values) {
					return nil, fmt.Errorf("invalid icon map %s", name.Name)
				}
				wanted[name.Name] = true
				m, ok := v.Values[i].(*ast.CompositeLit)
				if !ok || len(m.Elts) == 0 {
					return nil, fmt.Errorf("icon map %s is not a nonempty literal", name.Name)
				}
				for _, elem := range m.Elts {
					kv, ok := elem.(*ast.KeyValueExpr)
					if !ok {
						return nil, fmt.Errorf("icon map %s contains an unkeyed value", name.Name)
					}
					lit, ok := kv.Value.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						return nil, fmt.Errorf("icon map %s contains a nonliteral value", name.Name)
					}
					id, err := strconv.Unquote(lit.Value)
					if err != nil || !editorIconLike.MatchString(id) {
						return nil, fmt.Errorf("icon map %s contains invalid icon %s", name.Name, lit.Value)
					}
					out[id] = fset.Position(lit.Pos()).String()
				}
			}
		}
	}
	for name, seen := range wanted {
		if !seen {
			return nil, fmt.Errorf("icon map %s not found", name)
		}
	}
	return out, nil
}

func TestDocsIconMappingsCollector(t *testing.T) {
	const source = `package docs
	var taskIcons = map[string]string{"render2d.PostProcess": "render2d.normals", "timers": "code.timer"}
	var refIcons = map[string]string{"example": "cat.camera"}
	var kindIcons = map[int]string{1: "node.empty"}
	var unrelated = map[string]string{"ignored": "ui.missing"}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := docsIconMappings(fset, f, "taskIcons", "refIcons", "kindIcons")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"render2d.normals", "code.timer", "cat.camera", "node.empty"}
	if len(got) != len(want) {
		t.Fatalf("map values = %v, want %v", got, want)
	}
	for _, id := range want {
		if _, ok := got[id]; !ok {
			t.Errorf("map value %q was not collected", id)
		}
	}
}

func TestDocsIconMappingsCollectorRejectsOpaqueValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
	}{
		{"missing map", `package docs; var taskIcons = map[string]string{"x":"help.book"}; var refIcons = map[string]string{"x":"help.book"}`},
		{"nonliteral", `package docs; var taskIcons = map[string]string{"x": dynamic}; var refIcons = map[string]string{"x":"help.book"}; var kindIcons = map[int]string{1:"help.book"}`},
		{"invalid icon", `package docs; var taskIcons = map[string]string{"x":"not an icon"}; var refIcons = map[string]string{"x":"help.book"}; var kindIcons = map[int]string{1:"help.book"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "fixture.go", tc.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := docsIconMappings(fset, f, "taskIcons", "refIcons", "kindIcons"); err == nil {
				t.Fatal("collector accepted an unauditable icon map")
			}
		})
	}
}

func TestEditorIconCollectorIncludesNumberedModuleKeys(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "fixture.go", `package example; func fieldIcon() string { return "render2d.normals" }`, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	collectEditorIconUses(fset, f, found)
	if _, ok := found["render2d.normals"]; !ok {
		t.Fatal("module icon with a numeric prefix was not collected")
	}
}

func TestIconsIncludeDocumentationUses(t *testing.T) {
	rep, err := iconReport()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"animation.animator":   "Documentation navigation: animator",
		"animation.player":     "Documentation navigation: animation guide and settings",
		"cat.camera":           "Documentation navigation: camera group and script API",
		"cat.physics":          "Documentation navigation: physics group, script API and settings",
		"cat.transform":        "Documentation navigation: math script API",
		"node.empty":           "Documentation navigation: components group and node reference fields",
		"render2d.normals":     "Documentation navigation: normal mapping",
		"render2d.point_light": "Documentation navigation: platformer effects",
	}
	for _, row := range rep.Rows() {
		if by, ok := want[row.ID]; ok && row.Status == "ok" && slices.Contains(row.UsedBy, by) {
			delete(want, row.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("documentation icon uses missing from the composed audit: %v", want)
	}
}

// Read the website's actual map keys and values, without importing its separate
// Go module or depending on an application-layer package from engine/editor.
func websiteIconMappings(f *ast.File, name string) (map[string]string, error) {
	for _, decl := range f.Decls {
		group, ok := decl.(*ast.GenDecl)
		if !ok || group.Tok != token.VAR {
			continue
		}
		for _, spec := range group.Specs {
			v := spec.(*ast.ValueSpec)
			for i, n := range v.Names {
				if n.Name != name || i >= len(v.Values) {
					continue
				}
				m, ok := v.Values[i].(*ast.CompositeLit)
				if !ok || len(m.Elts) == 0 {
					return nil, fmt.Errorf("website icon map %s is not a nonempty literal", name)
				}
				out := map[string]string{}
				for _, elem := range m.Elts {
					kv, ok := elem.(*ast.KeyValueExpr)
					if !ok {
						return nil, fmt.Errorf("website icon map %s contains an unkeyed entry", name)
					}
					key, keyOK := kv.Key.(*ast.BasicLit)
					value, valueOK := kv.Value.(*ast.BasicLit)
					if !keyOK || !valueOK || key.Kind != token.STRING || value.Kind != token.STRING {
						return nil, fmt.Errorf("website icon map %s contains nonliteral entries", name)
					}
					k, err := strconv.Unquote(key.Value)
					if err != nil {
						return nil, err
					}
					id, err := strconv.Unquote(value.Value)
					if err != nil || !editorIconLike.MatchString(id) {
						return nil, fmt.Errorf("website icon map %s has invalid icon %s", name, value.Value)
					}
					if _, dup := out[k]; dup {
						return nil, fmt.Errorf("website icon map %s repeats %q", name, k)
					}
					out[k] = id
				}
				return out, nil
			}
		}
	}
	return nil, fmt.Errorf("website icon map %s not found", name)
}

func TestBuiltinNavigationIconPolicyMatchesWebsite(t *testing.T) {
	path := filepath.Join("..", "..", "tools", "gendocs", "site", "icons.go")
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	tasks, refs := docicons.Builtin()
	for _, tc := range []struct {
		name string
		got  map[string]string
	}{{"taskIcons", tasks}, {"refIcons", refs}} {
		want, err := websiteIconMappings(f, tc.name)
		if err != nil {
			t.Fatal(err)
		}
		if !maps.Equal(tc.got, want) {
			t.Errorf("%s differs from the website: app=%v website=%v", tc.name, tc.got, want)
		}
	}
}

func TestDocsNavigationIconsAreListed(t *testing.T) {
	found := map[string]string{}
	for _, src := range []struct {
		path  string
		names []string
	}{
		{filepath.Join("..", "..", "app", "docicons", "navigation.go"), []string{"taskIcons", "refIcons"}},
		{filepath.Join("..", "..", "editor", "panels", "docs", "icons.go"), []string{"kindIcons"}},
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, src.path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		icons, err := docsIconMappings(fset, f, src.names...)
		if err != nil {
			t.Fatal(err)
		}
		for id, at := range icons {
			found[id] = at
		}
	}
	for _, g := range regdocs.Groups() {
		if g.Icon != "" {
			found[g.Icon] = "documentation group " + g.ID
		}
	}
	listed := map[string]bool{}
	for _, u := range editorIcons() {
		listed[u.ID] = true
	}
	rep, err := iconReport()
	if err != nil {
		t.Fatal(err)
	}
	provided := map[string]bool{}
	for _, row := range rep.Rows() {
		provided[row.ID] = row.Status != "missing"
	}
	for id, at := range found {
		drawn := id
		// Only code.timer is an explicitly optional website icon in the panel:
		// when it is not baked, availableIcon uses play.speed. If it becomes
		// available, its own use must be registered instead.
		if id == "code.timer" && !provided[id] {
			drawn = "play.speed"
		}
		if !provided[drawn] {
			t.Errorf("%s uses unprovided documentation icon %s", at, drawn)
		}
		if !listed[drawn] {
			t.Errorf("%s uses documentation icon %s, add it to the appropriate icon audit", at, drawn)
		}
	}

	// Built-in docs can also supply group and entry icons as metadata. They
	// must be provided and used in the composed audit; plug-in icons remain
	// dynamic and retain the panel's normal missing-icon fallback.
	reg, err := docstool.Registry(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	tasks, refs := docicons.Builtin()
	for id, want := range tasks {
		if got := reg.NavigationTaskIcon(id); got != want {
			t.Errorf("composed registry task %q = %q, want %q", id, got, want)
		}
	}
	for id, want := range refs {
		if got := reg.NavigationReferenceIcon(id); got != want {
			t.Errorf("composed registry reference %q = %q, want %q", id, got, want)
		}
	}
	for _, id := range []string{"animation", "animation/animator", "render2d.PostProcess", "camera.CameraBrain2D", "aego/audio", "aego/timer", "section/animation"} {
		if _, ok := reg.Get(id); !ok {
			t.Errorf("composed registry does not contain module documentation entry %q", id)
		}
	}
	used := map[string]bool{}
	for _, row := range rep.Rows() {
		used[row.ID] = row.Status == "ok" && len(row.UsedBy) > 0
	}
	check := func(id, at string) {
		if id == "" {
			return
		}
		if !provided[id] || !used[id] {
			t.Errorf("%s uses documentation metadata icon %s, which is not provided and registered", at, id)
		}
	}
	for _, g := range reg.Groups() {
		check(g.Icon, "documentation group "+g.ID)
	}
	for _, e := range reg.All() {
		check(e.Icon, "documentation entry "+e.ID)
	}
}
