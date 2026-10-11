package langsvc

import (
	"slices"
	"strings"
	"testing"
)

const asyncTmpl = `package c

script Mover {
	signal Hit(amount int)

	var task Coroutine

	func Blink(times int) coroutine {
		yield
	}

	func OnHit(amount int) {
	}

	func Other() {
	}

	func Update(dt float) {
CURSOR
	}
}
`

func TestAsyncCompletion(t *testing.T) {
	s, w, _ := setup(t)
	cases := []struct {
		line string
		want []string
	}{
		{"\t\ttask = start |", []string{"Blink"}},
		{"\t\tyield |", []string{"wait", "frames", "fixed"}},
		{"\t\tHit.|", []string{"Emit", "Connect"}},
		{"\t\tx := node.GetScript<|", []string{"Mover"}},
		{"\t\ttask.|", []string{"Stop", "Running"}},
	}
	for _, tc := range cases {
		id, off := overlay(t, s, w, strings.Replace(asyncTmpl, "CURSOR", tc.line, 1))
		got := labels(s.CompleteNow(id, off))
		for _, want := range tc.want {
			if !slices.Contains(got, want) {
				t.Errorf("%q: missing %s in %v", tc.line, want, got)
			}
		}
	}
	id, off := overlay(t, s, w, strings.Replace(asyncTmpl, "CURSOR", "\t\tHit.Connect(|", 1))
	got := labels(s.CompleteNow(id, off))
	on, other := slices.Index(got, "OnHit"), slices.Index(got, "Other")
	if on < 0 || other < 0 || on > other {
		t.Fatalf("matching handler must come first: %v", got)
	}
	if slices.Contains(got, "Blink") || slices.Contains(got, "Update") {
		t.Fatalf("coroutines and lifecycle methods offered as handlers: %v", got)
	}
	id, off = overlay(t, s, w, strings.Replace(asyncTmpl, "\tsignal Hit(amount int)\n", "\tsignal Hit(amount int)\n\t|\n", 1))
	got = labels(s.CompleteNow(id, off))
	for _, want := range []string{"coroutine", "signal", "Reloaded"} {
		if !slices.Contains(got, want) {
			t.Errorf("script body: missing snippet %s", want)
		}
	}
}

func TestAsyncHoverAndSignature(t *testing.T) {
	s, w, _ := setup(t)
	src := strings.Replace(asyncTmpl, "CURSOR", "\t\ttask = start Blink(2)\n\t\tHit.Emit(1)", 1)
	id, _ := overlay(t, s, w, src+"|")
	at := func(sub string, delta int) uint32 { return uint32(strings.Index(src, sub) + delta) }
	cases := []struct {
		off  uint32
		want string
	}{
		{at("yield", 1), "yield frames(n)"},
		{at("coroutine {", 2), "start"},
		{at("start Blink", 1), "returns its Coroutine"},
		{at("Emit(1)", 1), "synchronously"},
		{at("signal Hit", 8), "signal Hit(amount int)"},
	}
	for _, tc := range cases {
		h, ok := s.Hover(id, tc.off)
		if !ok || !strings.Contains(h.Signature+"\n"+h.Doc, tc.want) {
			t.Errorf("hover at %d = %+v, want %q", tc.off, h, tc.want)
		}
	}
	for _, tc := range []struct {
		line string
		want string
	}{
		{"\t\tHit.Emit(|", "Emit(amount int)"},
		{"\t\tHit.Connect(|", "Connect(handler func(amount int)) Connection"},
		{"\t\ttask = start Blink(|", "times int"},
	} {
		full := strings.Replace(asyncTmpl, "CURSOR", tc.line, 1)
		id, off := overlay(t, s, w, full)
		text := strings.Replace(full, "|", "", 1)
		sig, ok := s.SignatureAt(s.Latest(), id, []byte(text), int(off))
		if !ok || !strings.Contains(sig.Label, tc.want) {
			t.Errorf("%q: signature %+v", tc.line, sig)
		}
	}
}

func TestAsyncTokens(t *testing.T) {
	s, w, _ := setup(t)
	src := strings.Replace(asyncTmpl, "CURSOR", "\t\ttask = start Blink(2)\n\t\tHit.Emit(1)", 1)
	id, _ := overlay(t, s, w, src+"|")
	kinds := map[uint32]TokenKind{}
	for _, tk := range s.SemanticTokens(id) {
		kinds[tk.Span.Start] = tk.Kind
	}
	want := map[int]TokenKind{
		strings.Index(src, "Hit(amount"):  TokSignal,
		strings.Index(src, "Hit.Emit"):    TokSignal,
		strings.Index(src, "Blink(times"): TokCoroutine,
		strings.Index(src, "Blink(2)"):    TokCoroutine,
	}
	for off, k := range want {
		if kinds[uint32(off)] != k {
			t.Errorf("token at %d: %d, want %d", off, kinds[uint32(off)], k)
		}
	}
}
