package docs

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"aego/engine/builtin"
	"aego/engine/module"
	"aego/script/bind"
	"aego/script/bind/manifest"
	"aego/script/diag"
	scriptmodule "aego/script/module"
	"aego/script/stdlib"
)

func build(t *testing.T, extra ...module.DocSource) (*Registry, *manifest.Manifest) {
	t.Helper()
	types, err := builtin.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	reg := bind.New()
	man, err := builtin.Load(types, reg, stdlib.Options{Seed: 1, Testing: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	if err := scriptmodule.Freeze(reg, types.Schema()); err != nil {
		t.Fatal(err)
	}
	bm, err := manifest.FromRegistry(reg, types.Schema())
	if err != nil {
		t.Fatal(err)
	}
	man.Docs = append(man.Docs, extra...)
	return FromManifest(bm, types.Schema(), man), bm
}

func TestCoverage(t *testing.T) {
	r, bm := build(t)
	want := func(id string, k Kind) {
		t.Helper()
		e, ok := r.Get(id)
		if !ok {
			t.Errorf("no entry %s", id)
			return
		}
		if e.Kind != k {
			t.Errorf("%s is %s, want %s", id, e.Kind, k)
		}
	}
	for _, m := range bm.Modules {
		want(m.Path, Module)
		for _, f := range m.Funcs {
			want(m.Path+"."+f.Name, Function)
		}
		for _, c := range m.Consts {
			want(m.Path+"."+c.Name, Const)
		}
	}
	for _, e := range bm.Enums {
		want(e.Module+"."+e.Name, Enum)
	}
	for _, me := range bm.Methods {
		want(BuiltinID+"."+me.Recv.String()+"."+me.Name, Method)
	}
	for _, c := range bm.Components {
		want(c.Name, Type)
		for _, f := range c.Fields {
			want(c.Name+"."+f.Name, Field)
		}
	}
	for _, c := range diag.Codes() {
		want(c, Code)
	}
	for _, p := range r.Check(false) {
		if p.Error {
			t.Errorf("%s: %s", p.ID, p.Message)
		}
	}
}

func TestSearchClamp(t *testing.T) {
	r, _ := build(t)
	hits := r.Search("clamp", 5)
	if len(hits) == 0 || hits[0].ID != "aego/math.Clamp" {
		ids := make([]string, len(hits))
		for i, h := range hits {
			ids[i] = h.ID
		}
		t.Fatalf("Search(clamp) = %v", ids)
	}
}

func TestPages(t *testing.T) {
	src := strings.Join([]string{
		"---",
		"id: test/coroutines",
		"title: Coroutines",
		"order: 2",
		`requires: [script, "render2d"]`,
		"---",
		"# Coroutines",
		"",
		"A coroutine pauses with [yield](ref:test/yield) and resumes later.",
		"",
		"```aes run",
		"package demo",
		"---",
		"```",
		"---",
		"id: test/yield",
		"parent: test/coroutines",
		`keys: ["test yield", "test yield, again"]`,
		"---",
		"yield pauses the current coroutine.",
		"---",
		"id: aego/math.Clamp",
		"---",
		"Extra text for [nothing](ref:missing/entry).",
		"",
	}, "\n")
	fsys := fstest.MapFS{"lang.md": {Data: []byte(src)}}
	r, _ := build(t, module.DocSource{Owner: "script", FS: fsys})

	page, ok := r.Get("test/coroutines")
	if !ok {
		t.Fatal("page missing")
	}
	if page.Summary != "A coroutine pauses with yield and resumes later." {
		t.Errorf("summary = %q", page.Summary)
	}
	if len(page.Snippets) != 1 || !page.Snippets[0].Run || page.Snippets[0].Lang != "aes" || page.Snippets[0].Code != "package demo\n---" {
		t.Errorf("snippets = %+v", page.Snippets)
	}
	if len(page.Requires) != 2 || page.Requires[1] != "render2d" {
		t.Errorf("requires = %v", page.Requires)
	}
	if e, ok := r.ByKey("test yield, again"); !ok || e.ID != "test/yield" {
		t.Errorf("ByKey = %v %v", e.ID, ok)
	}
	if kids := r.Children("test/coroutines"); len(kids) != 1 || kids[0].ID != "test/yield" {
		t.Errorf("children = %v", kids)
	}
	clamp, _ := r.Get("aego/math.Clamp")
	if clamp.Kind != Function || !strings.Contains(clamp.Body, "Extra text") {
		t.Errorf("clamp not merged: %+v", clamp)
	}
	broken := false
	for _, p := range r.Check(false) {
		if p.Error && p.ID == "aego/math.Clamp" && strings.Contains(p.Message, "missing/entry") {
			broken = true
		}
	}
	if !broken {
		t.Error("broken ref link is not reported")
	}
}

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

func TestKeywordsMatchLegacy(t *testing.T) {
	r, _ := build(t)
	data, err := os.ReadFile("testdata/keywords.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]string
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	got := r.Keywords()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("hover text of %q changed:\n got %q\nwant %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("%d keywords, want %d", len(got), len(want))
	}
}

func TestCompose(t *testing.T) {
	r, _ := build(t)
	md := r.Compose("language")
	for _, want := range []string{"# Aego Script\n", "\n## Coroutines\n", "\n### yield wait\n"} {
		if !strings.Contains(md, want) {
			t.Errorf("Compose(language) has no %q", want)
		}
	}
}
