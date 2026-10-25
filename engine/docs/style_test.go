//go:build !aego_runtime

package docs

import (
	"testing"
	"testing/fstest"

	"aego/engine/module"
)

func TestTrimSelf(t *testing.T) {
	for _, c := range []struct{ name, doc, want string }{
		{"ApplyForce", "ApplyForce pushes the body.", "Pushes the body."},
		{"OnCollisionEnter", "OnCollisionEnter runs after the first contact.", "Runs after the first contact."},
		{"Mass", "Mass in kilograms.", "Mass in kilograms."},
		{"Speed", "Speed of the body.", "Speed of the body."},
		{"Play", "Playing is true.", "Playing is true."},
		{"Grounded", "Grounded is true on the floor.", "Is true on the floor."},
		{"Stop", "Stop", "Stop"},
	} {
		if got := TrimSelf(c.name, c.doc); got != c.want {
			t.Errorf("TrimSelf(%q, %q) = %q, want %q", c.name, c.doc, got, c.want)
		}
	}
}

func TestFirstSentence(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"One. Two.", "One."},
		{"Uses e.g. a body. Then more.", "Uses e.g. a body."},
		{"Version 1.5 ships. Next.", "Version 1.5 ships."},
		{"No end", "No end"},
	} {
		if got := firstSentence(c.in); got != c.want {
			t.Errorf("firstSentence(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestProseChecks(t *testing.T) {
	ps := prose("Intro text.\n\n```aes\nsimply code\n```\n\n**Keyframes.** The editor keeps them.\n\n- item `just` here\n")
	if len(ps) != 3 {
		t.Fatalf("%d paragraphs, want 3: %+v", len(ps), ps)
	}
	if !pseudoHead.MatchString(ps[1].raw) || pseudoHead.MatchString("**Window → Theme** opens the menu.") {
		t.Fatal("pseudo-heading detection is wrong")
	}
	rules, err := lintRules()
	if err != nil || len(rules) == 0 {
		t.Fatalf("lint.aetext: %d rules, %v", len(rules), err)
	}
	for _, r := range rules {
		if r.re.MatchString(ps[2].text) {
			t.Fatalf("pattern %q found inside backticks: %q", r.src, ps[2].text)
		}
	}
}

func styleCodes(r *Registry, id string) map[string]int {
	out := map[string]int{}
	for _, p := range r.StyleProblems(false) {
		if p.ID == id {
			out[p.Message[:6]]++
		}
	}
	return out
}

func TestStyleProblems(t *testing.T) {
	long := "The editor keeps a copy of the whole game every few seconds while you play the scene."
	src := module.DocSource{Owner: "game", FS: fstest.MapFS{
		"a.md": page("---\nid: a\ngroup: start\norder: 1\n---\n\n# A\n\n" + long + " It simply works, see D123. In this stage we added `NoSuchThing()`.\n\n**Keyframes.** The editor keeps them.\n\n```go\nfunc broken( {\n```\n"),
		"b.md": page("---\nid: b\ngroup: start\norder: 2\nstyle_allow: [just]\n---\n\n# B\n\n" + long + " Press it just once.\n"),
		"c.md": page("---\nid: c\ngroup: start\norder: 3\n---\n\n# C\n\n## Steps\n\nDo it.\n"),
	}}
	r := Assemble(Sources{Docs: []module.DocSource{src}})
	a, b, c := styleCodes(r, "a"), styleCodes(r, "b"), styleCodes(r, "c")
	for _, code := range []string{"DOC010", "DOC011", "DOC013", "DOC020", "DOC022"} {
		if a[code] == 0 {
			t.Errorf("page a: no %s in %v", code, a)
		}
	}
	if a["DOC017"]+b["DOC017"] != 1 {
		t.Errorf("the shared sentence is reported %d times, want once: a %v b %v", a["DOC017"]+b["DOC017"], a, b)
	}
	if b["DOC010"] != 0 || a["DOC019"] != 0 || c["DOC019"] != 1 {
		t.Errorf("b allows just; a has an opening paragraph, c has none: a %v b %v c %v", a, b, c)
	}
}

func TestDiagnosticShape(t *testing.T) {
	var got []string
	add := func(_ *Entry, _ int, code, msg string) { got = append(got, code+" "+msg) }
	r := &Registry{}
	r.diagShape(&Entry{Kind: Code, Body: "A name is used twice.\n\n**Fix.** Rename one of them.\n\n```aes\nx\n```\n"}, add)
	if len(got) != 0 {
		t.Fatalf("a well-formed page is reported: %v", got)
	}
	r.diagShape(&Entry{Kind: Code, Body: "A name is used twice. It breaks things.\n\nFix: Rename one. Or delete it.\n"}, add)
	if len(got) != 2 {
		t.Fatalf("two long parts give %d problems, want 2: %v", len(got), got)
	}
}

func TestBrokenLinkIsAnError(t *testing.T) {
	src := module.DocSource{Owner: "game", FS: fstest.MapFS{
		"a.md": page("---\nid: a\ngroup: start\norder: 1\n---\n\n# A\n\nSee [gone](ref:gone).\n"),
	}}
	for _, p := range Assemble(Sources{Docs: []module.DocSource{src}}).Check(false) {
		if p.ID == "a" && p.Error && p.Message == "broken link ref:gone" {
			return
		}
	}
	t.Fatal("a link to a page that does not exist is not an error")
}
