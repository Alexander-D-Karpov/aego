package aetext

import (
	"strings"
	"testing"
)

const sample = `scene version=1 {
  node "3f9a0c21d4e84b7d9a1f2b3c4d5e6f70" name="Player" enabled=true layers=1 {
    transform {
      position=[100, 200]
      rotation=0
      scale=[1, 1]
    }
    component "game.Health" version=1 {
      max=100
      current=72.5
      tint=#ffcc00ff
      mode=nearest
      sprite=asset("8a9e4d2c1b0f4a3e9c7d6b5a4f3e2d1c")
      target=node("b17e5a3c9d2f4e6a8b0c1d2e3f405162")
      empty=[]
      nested={ a=1 b=false }
    }
  }
}
`

func TestParseStructure(t *testing.T) {
	doc, err := Parse([]byte(sample), "sample.aescene")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Head != "scene" {
		t.Fatalf("head = %q", doc.Head)
	}
	if v, ok := doc.Attr("version"); !ok || v.AsInt() != 1 {
		t.Fatal("version attribute missing")
	}
	node := doc.Block("node", "")
	if node == nil || node.Label != "3f9a0c21d4e84b7d9a1f2b3c4d5e6f70" {
		t.Fatal("node block missing or mislabelled")
	}
	if v, _ := node.Attr("name"); v.Str != "Player" {
		t.Fatalf("name = %q", v.Str)
	}
	comp := node.Block("component", "game.Health")
	if comp == nil {
		t.Fatal("component block missing")
	}
	checks := []struct {
		field string
		test  func(Value) bool
	}{
		{"max", func(v Value) bool { return v.Kind == VInt && v.Int == 100 }},
		{"current", func(v Value) bool { return v.Kind == VFloat && v.Float == 72.5 }},
		{"tint", func(v Value) bool { return v.Kind == VColor && v.Color == 0xffcc00ff }},
		{"mode", func(v Value) bool { return v.Kind == VIdent && v.Ident == "nearest" }},
		{"sprite", func(v Value) bool { return v.Kind == VAsset && len(v.Str) == 32 }},
		{"target", func(v Value) bool { return v.Kind == VNode && len(v.Str) == 32 }},
		{"empty", func(v Value) bool { return v.Kind == VList && len(v.List) == 0 }},
		{"nested", func(v Value) bool { return v.Kind == VStruct && len(v.Obj) == 2 }},
	}
	for _, c := range checks {
		v, ok := comp.Field(c.field)
		if !ok {
			t.Fatalf("field %s missing", c.field)
		}
		if !c.test(v) {
			t.Fatalf("field %s has wrong value: %+v", c.field, v)
		}
	}
}

func TestRoundTripIsIdempotent(t *testing.T) {
	doc, err := Parse([]byte(sample), "sample")
	if err != nil {
		t.Fatal(err)
	}
	first := Write(doc)
	again, err := Parse(first, "sample")
	if err != nil {
		t.Fatalf("re-parse failed: %v\n%s", err, first)
	}
	second := Write(again)
	if string(first) != string(second) {
		t.Fatalf("writer is not idempotent:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestCanonicalFloats(t *testing.T) {
	n := &Node{Head: "t"}
	n.Fields = []Field{
		{Name: "a", Value: Float32(0.1)},
		{Name: "b", Value: Float64(0.1)},
		{Name: "c", Value: Float32(1)},
		{Name: "d", Value: Int(1)},
	}
	got := string(Write(n))
	want := "t {\n  a=0.1\n  b=0.1\n  c=1\n  d=1\n}\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	single := Float32(0.1)
	if strconv := string(Write(&Node{Head: "x", Fields: []Field{{Name: "v", Value: single}}})); !strings.Contains(strconv, "v=0.1\n") {
		t.Fatalf("float32 not formatted canonically: %s", strconv)
	}
}

func TestEmptyBlockInline(t *testing.T) {
	n := &Node{Head: "a", Blocks: []*Node{{Head: "b"}}}
	got := string(Write(n))
	if got != "a {\n  b {}\n}\n" {
		t.Fatalf("got %q", got)
	}
}

func TestStringEscapes(t *testing.T) {
	src := "x {\n  s=\"a\\\"b\\\\c\\nd\\te\\u00e9\"\n}\n"
	doc, err := Parse([]byte(src), "x")
	if err != nil {
		t.Fatal(err)
	}
	v, _ := doc.Field("s")
	if v.Str != "a\"b\\c\nd\te\u00e9" {
		t.Fatalf("decoded = %q", v.Str)
	}
	out := string(Write(doc))
	if !strings.Contains(out, `\"`) || !strings.Contains(out, `\\`) || !strings.Contains(out, `\n`) {
		t.Fatalf("escapes lost: %s", out)
	}
	back, err := Parse([]byte(out), "x")
	if err != nil {
		t.Fatal(err)
	}
	if bv, _ := back.Field("s"); bv.Str != v.Str {
		t.Fatalf("round trip changed string: %q", bv.Str)
	}
}

func TestErrorPositions(t *testing.T) {
	cases := []struct {
		src  string
		line int
	}{
		{"scene {\n  a=\n}\n", 3},
		{"scene {\n  a=1\n", 3},
		{"scene {\n  tint=#fff\n}\n", 2},
		{"scene {\n  s=\"unterminated\n}\n", 2},
		{"scene {\n  x=[1, 2\n}\n", 3},
		{"scene {\n  1=2\n}\n", 2},
	}
	for _, c := range cases {
		_, err := Parse([]byte(c.src), "t.aescene")
		if err == nil {
			t.Fatalf("expected an error for %q", c.src)
		}
		d, ok := err.(*Diag)
		if !ok {
			t.Fatalf("error is not a Diag: %T", err)
		}
		if d.Line() != c.line {
			t.Fatalf("for %q: line %d, want %d (%s)", c.src, d.Line(), c.line, d.Msg)
		}
		if !strings.HasPrefix(d.Error(), "t.aescene:") {
			t.Fatalf("diagnostic misses the file name: %s", d.Error())
		}
	}
}
