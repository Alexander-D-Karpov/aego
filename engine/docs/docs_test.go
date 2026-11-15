//go:build !aego_runtime

package docs

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"aego/engine/module"
)

func TestPlain(t *testing.T) {
	md := "Text with [Clamp](ref:aego/math.Clamp) and []int.\n\n```aes\nx := 1\n```"
	want := "Text with Clamp and []int.\n\nx := 1"
	if got := Plain(md); got != want {
		t.Errorf("Plain = %q, want %q", got, want)
	}
}

func TestKeywordID(t *testing.T) {
	for in, want := range map[string]string{
		"yield wait": "keyword/yield-wait",
		"@export":    "keyword/export-attr",
		"Get<T>":     "keyword/get",
		"NodeRef":    "keyword/noderef",
	} {
		if got := KeywordID(in); got != want {
			t.Errorf("KeywordID(%q) = %q, want %q", in, got, want)
		}
	}
}

func page(body string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(body)}
}

func TestModuleDeclaresGuideGroup(t *testing.T) {
	src := module.DocSource{Owner: "game", FS: fstest.MapFS{
		"a.md": page("---\nid: flight\ngroup: flight\ngroup_title: Flight\ngroup_order: 45\norder: 1\n---\n\n# Flight\n\nFly.\n"),
		"b.md": page("---\nid: wings\ngroup: flight\ngroup_title: Wings\norder: 2\n---\n\n# Wings\n\nFlap.\n"),
		"c.md": page("---\nid: lost\ngroup: nowhere\norder: 1\n---\n\n# Lost\n\nHere.\n"),
	}}
	r := Assemble(Sources{Docs: []module.DocSource{src}})
	var order []string
	for _, g := range r.Export("x").Groups {
		order = append(order, g.ID)
	}
	if !slices.Equal(order, []string{"flight", GroupScriptAPI, GroupDiagnostics}) {
		t.Fatalf("a declared guide group must come before the reference groups: %v", order)
	}
	g, ok := r.group("flight")
	if !ok || g.Title != "Flight" || g.Section != SectionGuides || g.Order != 45 {
		t.Fatalf("declared group %+v %v", g, ok)
	}
	problems := map[string]string{}
	for _, p := range r.Check(false) {
		if p.Error {
			problems[p.ID] = p.Message
		}
	}
	if problems["wings"] == "" || problems["lost"] == "" || problems["flight"] != "" {
		t.Fatalf("problems %v", problems)
	}
}

func TestGroupNavigationMatchesFullExport(t *testing.T) {
	src := module.DocSource{Owner: "game", FS: fstest.MapFS{
		"guide.md": page("---\nid: flight\ngroup: flight\ngroup_title: Flight\ngroup_order: 45\norder: 1\n---\n\n# Flight\n\nFly.\n\n---\nid: flight/wings\nparent: flight\n---\n\n# Wings\n\nFlap.\n"),
	}}
	r := Assemble(Sources{Docs: []module.DocSource{src}})
	want := r.Export("x").Groups
	got := r.GroupNavigation()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("navigation differs from full export: got %+v; want %+v", got, want)
	}
	if len(got) == 0 || got[0].ID != "flight" || len(got[0].Nav) != 1 || got[0].Nav[0].ID != "flight" || len(got[0].Nav[0].Children) != 1 || got[0].Nav[0].Children[0].ID != "flight/wings" {
		t.Fatalf("navigation lost the declared group or its child: %+v", got)
	}
	got[0].Nav[0].ID = "changed"
	if next := r.GroupNavigation(); next[0].Nav[0].ID != "flight" {
		t.Fatal("a caller changed future navigation")
	}
}

func TestScriptAPIIsAlphabeticalAcrossModulesAndMembers(t *testing.T) {
	r := &Registry{byID: map[string]int{}, children: map[string][]int{}, keys: map[string]int{}}
	for _, e := range []Entry{
		{ID: BuiltinID, Kind: Module, Group: GroupScriptAPI, Order: -1, Title: "Builtin types"},
		{ID: EventsID, Kind: Module, Group: GroupScriptAPI, Order: -1, Title: "Script events"},
		{ID: "aego/zebra", Kind: Module, Group: GroupScriptAPI, Order: 1, Title: "aego/zebra"},
		{ID: "aego/alpha", Kind: Module, Group: GroupScriptAPI, Order: 50, Title: "aego/alpha"},
		{ID: "aego/alpha.Zebra", Parent: "aego/alpha", Kind: Function, Title: "Zebra"},
		{ID: "aego/alpha.Apple", Parent: "aego/alpha", Kind: Type, Title: "Apple"},
		{ID: "aego/alpha.banana", Parent: "aego/alpha", Kind: Function, Title: "banana"},
	} {
		r.add(e)
	}
	r.index()
	ids := func(entries []Entry) []string {
		out := make([]string, len(entries))
		for i, e := range entries {
			out[i] = e.ID
		}
		return out
	}
	if got, want := ids(r.GroupRoots(GroupScriptAPI)), []string{BuiltinID, EventsID, "aego/alpha", "aego/zebra"}; !slices.Equal(got, want) {
		t.Errorf("script modules = %v, want %v", got, want)
	}
	if got, want := ids(r.Children("aego/alpha")), []string{"aego/alpha.Apple", "aego/alpha.banana", "aego/alpha.Zebra"}; !slices.Equal(got, want) {
		t.Errorf("script members = %v, want %v", got, want)
	}
}

func TestModuleCanDocumentItsGeneratedProjectSection(t *testing.T) {
	r := &Registry{byID: map[string]int{}}
	r.add(Entry{ID: "section/animation", Kind: Page, Owner: "animation", Group: GroupSettings, Body: "Generated introduction."})
	rec := record{id: "section/animation", body: "## Events\n\nDeclare animation events.", icon: "animation.player", line: 3}
	r.page("animation", "animation.md", "modules/animation/res/docs/animation.md", rec)
	e, _ := r.Get("section/animation")
	if !strings.Contains(e.Body, "Generated introduction.") || !strings.Contains(e.Body, "Declare animation events.") || e.Icon != "animation.player" {
		t.Fatalf("section documentation not merged: %+v", e)
	}
	if len(r.Problems()) != 0 {
		t.Fatalf("valid section documentation was rejected: %+v", r.Problems())
	}
	r.page("other", "other.md", "other.md", rec)
	if len(r.Problems()) == 0 {
		t.Fatal("a second or foreign declaration must not overwrite the section")
	}
}

func runProblems(r *Registry) map[string]bool {
	out := map[string]bool{}
	for _, p := range r.Check(false) {
		if p.Error && strings.Contains(p.Message, "runnable example") {
			out[p.ID] = true
		}
	}
	return out
}

func TestScriptDocsNeedRunBlocks(t *testing.T) {
	guide := "---\nid: lang\ngroup: script\norder: 1\n---\n\n# Lang\n\nText.\n\n" +
		"---\nid: lang/a\nparent: lang\n---\n\n# A\n\n```aes run\npackage demo\n```\n\n" +
		"---\nid: lang/b\nparent: lang\n---\n\n# B\n\n```aes\npackage demo\n```\n\n" +
		"---\nid: lang/kw\nparent: lang/b\nkeys: [kw]\n---\n\nkw is a keyword.\n"
	src := module.DocSource{Owner: scriptOwner, FS: fstest.MapFS{"lang.md": page(guide)}}
	got := runProblems(Assemble(Sources{Docs: []module.DocSource{src}}))
	if !got[BuiltinID] || !got["lang/b"] || got["lang"] || got["lang/a"] || got["lang/kw"] {
		t.Fatalf("missing run blocks reported for %v", got)
	}
	api := module.DocSource{Owner: scriptOwner, FS: fstest.MapFS{"api.md": page("---\nid: " + BuiltinID + "\n---\n\n```aes run\npackage demo\n```\n")}}
	if got := runProblems(Assemble(Sources{Docs: []module.DocSource{api}})); got[BuiltinID] {
		t.Fatalf("a module with a run block is still reported: %v", got)
	}
	project := module.DocSource{Owner: "game", FS: fstest.MapFS{"g.md": page("---\nid: mine\ngroup: script\norder: 2\n---\n\n# Mine\n")}}
	if got := runProblems(Assemble(Sources{Docs: []module.DocSource{project}})); got["mine"] {
		t.Fatalf("a project page is held to the run block rule: %v", got)
	}
}

func TestSummarySkipsImages(t *testing.T) {
	got := summary("# Editor\n\n![The Design workspace](shots/editor.png)\n\nThe editor has workspaces.\n")
	if got != "The editor has workspaces." {
		t.Fatalf("summary = %q", got)
	}
}

// A module code needs a page, a scene code needs an example, and a page needs the code in Info.Codes (D146).
func TestModuleCodePages(t *testing.T) {
	src := module.DocSource{Owner: "game", FS: fstest.MapFS{
		"diagnostics/GAM001.md":      page("---\ncode: GAM001\nseverity: warning\ntitle: Has an example\nexample: GAM001.aescene\n---\n\nText.\n"),
		"diagnostics/GAM001.aescene": page("scene version=1 {\n}\n"),
		"diagnostics/GAM002.md":      page("---\ncode: GAM002\nseverity: error\ntitle: Asset code\nscope: asset\n---\n\nText.\n"),
		"diagnostics/GAM003.md":      page("---\ncode: GAM003\nseverity: error\ntitle: No example\n---\n\nText.\n"),
		"diagnostics/GAM004.md":      page("---\ncode: GAM004\nseverity: error\ntitle: Undeclared\nscope: asset\n---\n\nText.\n"),
		"diagnostics/GAM006.md":      page("---\ncode: GAM006\nseverity: warning\ntitle: Only during Play\nscope: play\n---\n\nText.\n"),
	}}
	mods := []module.Info{{ID: "game", Codes: []string{"GAM001", "GAM002", "GAM003", "GAM005", "GAM006"}}}
	r := Assemble(Sources{Modules: mods, Docs: []module.DocSource{src}})
	got := map[string]bool{}
	for _, p := range r.Problems() {
		got[p.ID] = true
	}
	if got["GAM001"] || got["GAM002"] || got["GAM006"] || !got["GAM003"] || !got["GAM004"] || !got["GAM005"] {
		t.Fatalf("problems %v, want GAM003 (no example), GAM004 (undeclared) and GAM005 (no page)", got)
	}
	e, ok := r.Get("GAM001")
	if !ok || e.Kind != Code || e.Parent != DiagnosticsID || e.Diag.Example == "" || e.Diag.Severity != "warning" {
		t.Fatalf("GAM001 entry %+v", e)
	}
}

func TestReadsInput(t *testing.T) {
	for src, want := range map[string]bool{
		"package demo\n\nimport \"aego/input\"\n":                          true,
		"package demo\n\nimport (\n\t\"aego/math\"\n\t\"aego/input\"\n)\n": true,
		"package demo\n\nimport \"aego/math\"\n\n// aego/input later\n":    false,
		"package demo\n": false,
	} {
		if got := readsInput(src); got != want {
			t.Fatalf("readsInput(%q) = %v", src, got)
		}
	}
}

func TestTryKeys(t *testing.T) {
	src := "if input.KeyPressed(input.KeyJ) || input.Down(input.KeyA) || input.KeyReleased(input.KeySpace) {\n}\nx := input.KeyR"
	if got := strings.Join(extraKeys(src), ","); got != "J,Space,R" {
		t.Fatalf("extraKeys = %q", got)
	}
}
