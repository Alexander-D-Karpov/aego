package langsvc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	scriptstdlib "aego/modules/scripting/stdlib"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/scriptcomp"
	"aego/schema"
	"aego/script/bind"
	"aego/script/bind/manifest"
	"aego/script/source"
	"aego/script/stdlib"
	"aego/script/workspace"
)

const srcA = "package a\n\n// Add sums two ints.\nfunc Add(x int, y int) int {\n\treturn x + y\n}\n"
const srcB = "package b\n\nimport \"game/a\"\n\nfunc Twice(x int) int {\n\treturn a.Add(x, x)\n}\n"

const scriptTmpl = `package c

import (
	"aego/math"
	"game/combat"
)

script Mover {
	@export
	var Speed float = 1

	func Update(dt float) {
CURSOR
	}
}
`

type queue struct {
	work []func()
	main []func()
}

func (q *queue) Submit(fn func()) { q.work = append(q.work, fn) }
func (q *queue) OnMain(fn func()) { q.main = append(q.main, fn) }

func (q *queue) run() {
	for len(q.work) > 0 || len(q.main) > 0 {
		if len(q.work) > 0 {
			fn := q.work[0]
			q.work = q.work[1:]
			fn()
			continue
		}
		fn := q.main[0]
		q.main = q.main[1:]
		fn()
	}
}

func setup(t *testing.T, extra ...func(*bind.Registry)) (*Service, *workspace.Workspace, *queue) {
	t.Helper()
	root := t.TempDir()
	for rel, src := range map[string]string{"scripts/a/a.aes": srcA, "scripts/b/b.aes": srcB} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	types := scene.NewTypes(schema.NewRegistry())
	if err := frame.RegisterTypes(types); err != nil {
		t.Fatal(err)
	}
	if err := registerMarker(types); err != nil {
		t.Fatal(err)
	}
	if err := scriptcomp.Register(types); err != nil {
		t.Fatal(err)
	}
	r := bind.NewManifest()
	scriptstdlib.Install(r, stdlib.Options{Seed: 1, Testing: true})
	bind.Proc2(r.Module("game/combat"), "Damage", func(*bind.Ctx, int64, float32) {}, bind.Doc("deals damage"))
	for _, fn := range extra {
		fn(r)
	}
	m, err := manifest.FromRegistry(r, types.Schema())
	if err != nil {
		t.Fatal(err)
	}
	kw, err := os.ReadFile("../../tests/docs/testdata/keywords.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(kw, &m.Keywords); err != nil {
		t.Fatal(err)
	}
	tab, err := m.Table()
	if err != nil {
		t.Fatal(err)
	}
	w, err := workspace.Open(workspace.Config{Root: root, Module: "game", Bindings: tab, Docs: m})
	if err != nil {
		t.Fatal(err)
	}
	q := &queue{}
	return New(w, q), w, q
}

func overlay(t *testing.T, s *Service, w *workspace.Workspace, src string) (source.FileID, uint32) {
	t.Helper()
	off := strings.Index(src, "|")
	src = src[:off] + src[off+1:]
	w.SetOverlay("scripts/c/c.aes", []byte(src))
	s.Invalidate()
	s.Refresh()
	id, ok := w.FileID("scripts/c/c.aes")
	if !ok {
		t.Fatal("overlay file has no id")
	}
	return id, uint32(off)
}

func labels(c Completion) []string {
	out := make([]string, len(c.Items))
	for i, it := range c.Items {
		out[i] = it.Label
	}
	return out
}

func TestStaleCompletionDropped(t *testing.T) {
	s, w, q := setup(t)
	id, off := overlay(t, s, w, strings.Replace(scriptTmpl, "CURSOR", "\t\tx := math.|", 1))
	var got []string
	g1 := s.Invalidate()
	s.Complete(g1, id, off, func(Completion) { got = append(got, "first") })
	g2 := s.Invalidate()
	s.Complete(g2, id, off, func(Completion) { got = append(got, "second") })
	q.work[1]()
	q.work[0]()
	q.work = nil
	q.run()
	if !slices.Equal(got, []string{"second"}) {
		t.Fatalf("delivered %v", got)
	}
}

func TestCompletionContexts(t *testing.T) {
	s, w, _ := setup(t)
	cases := []struct {
		src  string
		want []string
	}{
		{strings.Replace(scriptTmpl, "CURSOR", "\t\tx := math.|", 1), []string{"Clamp", "Pi"}},
		{strings.Replace(scriptTmpl, "CURSOR", "\t\tx := node.|", 1), []string{"Position", "Get", "Has"}},
		{strings.Replace(scriptTmpl, "CURSOR", "\t\tx := node.Get<|", 1), []string{"Marker"}},
		{strings.Replace(scriptTmpl, "CURSOR", "\t\tx := spe|", 1), []string{"Speed"}},
		{strings.Replace(scriptTmpl, "CURSOR", "\t\tcombat.|", 1), []string{"Damage"}},
		{strings.Replace(scriptTmpl, "\t@export\n", "\t@|\n", 1), []string{"export", "former", "hide"}},
		{strings.Replace(scriptTmpl, "\t@export\n", "\t@export(|)\n", 1), []string{"min", "unit"}},
	}
	for _, tc := range cases {
		id, off := overlay(t, s, w, tc.src)
		got := labels(s.CompleteNow(id, off))
		for _, want := range tc.want {
			if !slices.Contains(got, want) {
				t.Errorf("%q: missing %s in %v", tc.want, want, got)
			}
		}
	}
}

func TestHoverNativeDoc(t *testing.T) {
	s, w, _ := setup(t)
	src := strings.Replace(scriptTmpl, "CURSOR", "\t\tcombat.Damage(1, 2)", 1)
	id, _ := overlay(t, s, w, src+"|")
	off := uint32(strings.Index(src, "Damage") + 2)
	h, ok := s.Hover(id, off)
	if !ok || h.Doc != "deals damage" || !strings.Contains(h.Signature, "game/combat.Damage(int, float)") {
		t.Fatalf("hover = %+v", h)
	}
}

func TestDefinitionReferencesHover(t *testing.T) {
	s, w, _ := setup(t)
	s.Refresh()
	b, _ := w.FileID("scripts/b/b.aes")
	a, _ := w.FileID("scripts/a/a.aes")
	use := uint32(strings.Index(srcB, "Add") + 1)
	def, ok := s.Definition(b, use)
	if !ok || def.Path != "scripts/a/a.aes" {
		t.Fatalf("definition = %+v", def)
	}
	if h, ok := s.Hover(b, use); !ok || h.Doc != "Add sums two ints." {
		t.Fatalf("hover = %+v", h)
	}
	refs := s.References(a, uint32(strings.Index(srcA, "Add(")+1))
	if len(refs) != 2 || refs[0].Path != "scripts/a/a.aes" || refs[1].Path != "scripts/b/b.aes" {
		t.Fatalf("references = %+v", refs)
	}
}

func TestDefinitionHostStub(t *testing.T) {
	s, w, _ := setup(t)
	cases := []struct {
		call, name, stub, symbol string
	}{
		{"combat.Damage(1, 2)", "Damage", StubPath("game/combat"), "func:Damage"},
		{"x := node.IsAlive()", "IsAlive", StubPath(StubBuiltin), "method:NodeRef.IsAlive"},
	}
	for _, tc := range cases {
		src := strings.Replace(scriptTmpl, "CURSOR", "\t\t"+tc.call, 1)
		id, _ := overlay(t, s, w, src+"|")
		def, ok := s.Definition(id, uint32(strings.Index(src, tc.name)+1))
		if !ok || !def.Native || def.Path != "" || def.Stub != tc.stub || def.Symbol != tc.symbol {
			t.Fatalf("%s: definition = %+v", tc.name, def)
		}
		st, ok := s.Stub(def.Stub)
		if !ok {
			t.Fatalf("%s: no stub %s", tc.name, def.Stub)
		}
		if _, ok := st.Lines[def.Symbol]; !ok {
			t.Fatalf("%s: stub has no line for %s", tc.name, def.Symbol)
		}
	}
}

func TestImplementationInNativeRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "native")
	strike := filepath.Join(root, "combat", "strike.go")
	s, w, _ := setup(t, func(r *bind.Registry) {
		m := r.Module("mygame/combat")
		bind.Proc1(m, "Strike", func(*bind.Ctx, int64) {})
		bind.Describe(m, "Strike", func(i *bind.Info) { i.Loc = strike + ":12" })
	})
	s.SetNativeRoot(root)
	src := strings.Replace(strings.Replace(scriptTmpl, "\"game/combat\"", "\"mygame/combat\"", 1),
		"CURSOR", "\t\tcombat.Strike(1)\n\t\tx := math.Clamp(1, 0, 2)", 1)
	id, _ := overlay(t, s, w, src+"|")
	if st, ok := s.Stub(StubPath("mygame/combat")); !ok || st.Locs["func:Strike"] != strike+":12" {
		t.Fatalf("stub loc = %v", st)
	}
	def, ok := s.Definition(id, uint32(strings.Index(src, "Strike")+1))
	if !ok || !def.Native || def.Path != "" || def.Stub != StubPath("mygame/combat") || def.Symbol != "func:Strike" {
		t.Fatalf("definition = %+v", def)
	}
	impl, ok := s.Implementation(def)
	if !ok || impl.Path != strike || impl.Line != 12 {
		t.Fatalf("implementation = %+v %v", impl, ok)
	}
	def, ok = s.Definition(id, uint32(strings.Index(src, "Clamp")+1))
	if !ok || !def.Native {
		t.Fatalf("clamp definition = %+v", def)
	}
	if impl, ok := s.Implementation(def); ok {
		t.Fatalf("an engine builtin resolved to %+v", impl)
	}
}

func TestSemanticTokens(t *testing.T) {
	s, w, _ := setup(t)
	s.Refresh()
	b, _ := w.FileID("scripts/b/b.aes")
	toks := s.SemanticTokens(b)
	kinds := map[string]TokenKind{}
	for i, tk := range toks {
		if i > 0 && toks[i-1].Span.Start >= tk.Span.Start {
			t.Fatal("tokens are not sorted")
		}
		kinds[srcB[tk.Span.Start:tk.Span.End]] = tk.Kind
	}
	want := map[string]TokenKind{"b": TokNamespace, "Twice": TokFunction, "Add": TokFunction, "x": TokParameter}
	for name, k := range want {
		if kinds[name] != k {
			t.Errorf("%s: kind %d, want %d", name, kinds[name], k)
		}
	}
}

func TestInlayHints(t *testing.T) {
	s, w, _ := setup(t)
	src := "package c\n\nfunc add(first int, second int) int {\n\treturn first + second\n}\n\nfunc f() int {\n\tfirst := 1\n\treturn add(first, 2)\n}\n|"
	id, _ := overlay(t, s, w, src)
	hs := s.HintsIn(s.Latest(), id)
	if len(hs) != 1 || hs[0].Label != "second:" {
		t.Fatalf("hints = %+v", hs)
	}
}

func TestStubs(t *testing.T) {
	s, _, _ := setup(t)
	s.Refresh()
	st, ok := s.Stub(StubPath("aego/math"))
	if !ok || !strings.Contains(st.Text, "func Clamp(v float, lo float, hi float) float") {
		t.Fatalf("math stub %v", ok)
	}
	if line := strings.Split(st.Text, "\n")[st.Lines["func:Clamp"]]; !strings.HasPrefix(line, "func Clamp(") {
		t.Fatalf("Clamp line = %q", line)
	}
	comps, ok := s.Stub(StubPath(StubComponents))
	if !ok || !strings.Contains(comps.Text, "component test.Marker {") {
		t.Fatal("components stub")
	}
	if _, ok := s.Stub(StubPath("nope/x")); ok {
		t.Fatal("stub for an unknown module")
	}
}
