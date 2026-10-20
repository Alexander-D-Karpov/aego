//go:build !aego_runtime

package docs

import (
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
