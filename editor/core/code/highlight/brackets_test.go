package highlight

import "testing"

func TestScanCarriesDepthAndComment(t *testing.T) {
	s := Scan([]byte("f(a, {"), Normal)
	if s.Depth() != 2 || s.Comment() {
		t.Fatalf("depth=%d comment=%v", s.Depth(), s.Comment())
	}
	s = Scan([]byte("}) /* x"), s)
	if s.Depth() != 0 || !s.Comment() {
		t.Fatalf("depth=%d comment=%v", s.Depth(), s.Comment())
	}
	s = Scan([]byte("*/ ("), s)
	if s.Depth() != 1 || s.Comment() {
		t.Fatalf("depth=%d comment=%v", s.Depth(), s.Comment())
	}
}

func TestTypeArgBrackets(t *testing.T) {
	brs, _ := Brackets([]byte("n.Get<Body>()"), Normal, nil)
	want := "<>()"
	if len(brs) != len(want) {
		t.Fatalf("brackets = %+v", brs)
	}
	for i := range brs {
		if brs[i].Ch != want[i] {
			t.Fatalf("bracket %d = %q", i, brs[i].Ch)
		}
	}
	if !brs[0].Open || brs[1].Open || brs[1].Depth != 0 {
		t.Fatalf("type args = %+v", brs[:2])
	}
	brs, _ = Brackets([]byte("if a<b { }"), Normal, nil)
	for _, b := range brs {
		if b.Ch == '<' {
			t.Fatal("comparison taken for a type argument")
		}
	}
}

func TestStringsAndCommentsIgnored(t *testing.T) {
	brs, s := Brackets([]byte(`f(")", "(") // )`), Normal, nil)
	if len(brs) != 2 || s.Depth() != 0 {
		t.Fatalf("brackets = %+v depth = %d", brs, s.Depth())
	}
}

func TestLineLevels(t *testing.T) {
	text := []byte("a(b[c])")
	spans, st := Line(text, Normal, nil)
	levels := BracketLevels(spans, text, 0, nil)
	var got []int
	for i, s := range spans {
		if s.Kind == Bracket {
			got = append(got, levels[i])
		}
	}
	want := []int{0, 1, 1, 0}
	if len(got) != len(want) || st.Depth() != 0 {
		t.Fatalf("levels = %v state = %d", got, st)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("levels = %v", got)
		}
	}
}
