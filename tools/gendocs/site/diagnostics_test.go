package site

import (
	"strings"
	"testing"
)

func diagFixture() *Export {
	exp := fixture()
	for i := range exp.Entries {
		if exp.Entries[i].ID == "AES2050" {
			exp.Entries[i].Summary = "yield outside a coroutine."
			exp.Entries[i].Diag = &DiagInfo{
				Phase: "Coroutines, signals and events", Fix: "Declare it with `coroutine`.", File: "demo/main.aes",
				Bad:      "package demo\n\nscript Blink {\n\tfunc Ready() {\n\t\tyield wait(0.5)\n\t}\n}\n",
				Good:     "package demo\n\nscript Blink {\n\tfunc Later() coroutine {\n\t\tyield wait(0.5)\n\t}\n}\n",
				Severity: "error", At: "demo/main.aes", Line: 5, Col: 3, EndLine: 5, EndCol: 18,
				Message: "yield is only allowed in a coroutine",
			}
		}
	}
	exp.Entries = append(exp.Entries,
		Entry{ID: "AES2031", Parent: "diagnostics", Kind: KindCode, Group: "diagnostics", Title: "AES2031", Summary: "Unused variable.",
			Diag: &DiagInfo{Phase: "Types", Fix: "Use it.", File: "demo/main.aes",
				Bad: "package demo\n\nfunc F() {\n\tx := 1\n}\n", Good: "package demo\n\nfunc F() {\n}\n",
				Severity: "warning", At: "demo/main.aes", Line: 4, Col: 2, EndLine: 4, EndCol: 3, Message: "x declared and not used"}},
		Entry{ID: "AES6002", Parent: "diagnostics", Kind: KindCode, Group: "diagnostics", Title: "AES6002", Summary: "A native binding panicked.",
			Diag: &DiagInfo{Phase: "Runtime faults", Fix: "Fix the Go function.", Why: "Raised when Go code panics."}},
	)
	for gi := range exp.Groups {
		if exp.Groups[gi].ID == "diagnostics" {
			nav := &exp.Groups[gi].Nav[0]
			nav.Children = []Node{{ID: "AES2031"}, {ID: "AES2050"}, {ID: "AES6002"}}
		}
	}
	return exp
}

func TestDiagnosticsPage(t *testing.T) {
	_, _, pages := build(t, diagFixture(), Options{})
	p := pages["diagnostics.html"]
	for _, want := range []string{
		`<a href="#phase-types">Types</a>`,
		`<a href="#AES2050">AES2050</a>`,
		`<h2 id="phase-coroutines-signals-and-events">Coroutines, signals and events</h2>`,
		`<h3 class="diag-head" id="AES2050"><a class="anchor" href="#AES2050">AES2050</a><span class="sev sev-error">error</span></h3>`,
		`<span class="sev sev-warning">warning</span>`,
		`<span class="hl-line">`,
		`<span class="hl-span"><span class="t-kw">yield</span> `,
		`<div class="diag-msg">demo/main.aes:5:3: AES2050 yield is only allowed in a coroutine</div>`,
		`<div class="diag-msg warning">demo/main.aes:4:2: AES2031 x declared and not used</div>`,
		`<strong>Fix:</strong> Declare it with <code>coroutine</code>.`,
		`<p class="label">Fixed</p>`,
		`<p class="label">No example</p>`,
		`Raised when Go code panics.`,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("diagnostics page has no %s", want)
		}
	}
	if strings.Count(p, `class="hl-line"`) != 2 {
		t.Errorf("want one marked line per broken example, got %d", strings.Count(p, `class="hl-line"`))
	}
	if i, j := strings.Index(p, `id="AES2031"`), strings.Index(p, `id="AES2050"`); i < 0 || j < i {
		t.Error("codes are not grouped in order")
	}
}

func TestHighlightMarked(t *testing.T) {
	got := highlightMarked("a := \"жx\"\nb := 1\n", 1, 8, 1, 9)
	if !strings.HasPrefix(got, `<span class="hl-line">`) || !strings.Contains(got, `&#34;ж</span><span class="hl-span"><span class="t-str">x</span></span>`) {
		t.Fatalf("marked line: %s", got)
	}
	if strings.Count(got, "hl-line") != 1 || !strings.Contains(got, "</span></span>\nb ") {
		t.Fatalf("other lines: %s", got)
	}
	if got := highlightMarked("x\n", 1, 0, 0, 0); strings.Contains(got, "hl-span") || !strings.Contains(got, "hl-line") {
		t.Fatalf("fault without a column: %s", got)
	}
}
