package lexer

import (
	"testing"

	"aego/script/diag"
	"aego/script/token"
)

func kinds(src string) []token.Kind {
	toks, _ := Tokenize(1, []byte(src))
	out := make([]token.Kind, len(toks))
	for i, t := range toks {
		out[i] = t.Kind
	}
	return out
}

func equal(a, b []token.Kind) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func codes(src string) []string {
	_, ds := Tokenize(1, []byte(src))
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Code
	}
	return out
}

func TestOperators(t *testing.T) {
	src := "+ - * / % == != < <= > >= && || ! = := += -= *= /= %= ++ -- . , ; : ( ) [ ] { } @"
	want := []token.Kind{
		token.Add, token.Sub, token.Mul, token.Quo, token.Rem, token.Eq, token.Ne,
		token.Lt, token.Le, token.Gt, token.Ge, token.AndAnd, token.OrOr, token.Not,
		token.Assign, token.Define, token.AddAssign, token.SubAssign, token.MulAssign,
		token.QuoAssign, token.RemAssign, token.Inc, token.Dec, token.Period, token.Comma,
		token.Semicolon, token.Colon, token.LParen, token.RParen, token.LBrack, token.RBrack,
		token.LBrace, token.RBrace, token.At, token.EOF,
	}
	if got := kinds(src); !equal(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestSemicolonInsertion(t *testing.T) {
	S := token.Semicolon
	cases := []struct {
		src  string
		want []token.Kind
	}{
		{"x\n", []token.Kind{token.Ident, S, token.EOF}},
		{"1\n", []token.Kind{token.Int, S, token.EOF}},
		{"1.5", []token.Kind{token.Float, S, token.EOF}},
		{"\"s\"\n", []token.Kind{token.String, S, token.EOF}},
		{"f()\n", []token.Kind{token.Ident, token.LParen, token.RParen, S, token.EOF}},
		{"a[0]\n", []token.Kind{token.Ident, token.LBrack, token.Int, token.RBrack, S, token.EOF}},
		{"{\n}\n", []token.Kind{token.LBrace, token.RBrace, S, token.EOF}},
		{"return\n", []token.Kind{token.Return, S, token.EOF}},
		{"break\ncontinue\n", []token.Kind{token.Break, S, token.Continue, S, token.EOF}},
		{"i++\nj--\n", []token.Kind{token.Ident, token.Inc, S, token.Ident, token.Dec, S, token.EOF}},
		{"true\nnil\n", []token.Kind{token.True, S, token.Nil, S, token.EOF}},
		{"x +\ny", []token.Kind{token.Ident, token.Add, token.Ident, S, token.EOF}},
		{"x // c\n", []token.Kind{token.Ident, S, token.EOF}},
		{"x /* a\nb */ y", []token.Kind{token.Ident, S, token.Ident, S, token.EOF}},
		{"x /* a */ y", []token.Kind{token.Ident, token.Ident, S, token.EOF}},
		{"var\n", []token.Kind{token.Var, token.EOF}},
		{"x;\n", []token.Kind{token.Ident, S, token.EOF}},
	}
	for _, c := range cases {
		if got := kinds(c.src); !equal(got, c.want) {
			t.Errorf("%q: got %v want %v", c.src, got, c.want)
		}
	}
}

func TestAutoSemicolonText(t *testing.T) {
	toks, _ := Tokenize(1, []byte("x\ny;"))
	if !toks[1].IsAuto() || toks[3].IsAuto() {
		t.Fatal("auto and explicit semicolons must be distinguishable")
	}
	if toks[1].Start != 1 || toks[1].End != 1 {
		t.Fatalf("auto semicolon span %d-%d", toks[1].Start, toks[1].End)
	}
}

func TestKeywordsAndReserved(t *testing.T) {
	for _, kw := range []string{"package", "import", "script", "type", "struct", "enum", "var", "const", "func",
		"return", "if", "else", "for", "switch", "case", "default", "break", "continue", "true", "false", "nil"} {
		if k := token.Lookup(kw); !k.IsKeyword() {
			t.Errorf("%s is not a keyword", kw)
		}
	}
	for _, rw := range []string{"yield", "signal", "coroutine", "await", "select", "go", "chan", "defer", "interface", "map"} {
		toks, ds := Tokenize(1, []byte(rw))
		if len(ds) != 1 || ds[0].Code != diag.ReservedWord {
			t.Errorf("%s: diagnostics %v", rw, ds)
		}
		if toks[0].Kind != token.Ident {
			t.Errorf("%s must continue as an identifier", rw)
		}
	}
}

func TestNumbers(t *testing.T) {
	ints := map[string]uint64{"10": 10, "0x1f": 31, "0X1F": 31, "1_000": 1000, "9223372036854775808": 1 << 63}
	for src, want := range ints {
		toks, ds := Tokenize(1, []byte(src))
		if len(ds) > 0 || toks[0].Kind != token.Int {
			t.Errorf("%s: %v %v", src, toks[0], ds)
			continue
		}
		if v, ok := ParseInt(toks[0].Text); !ok || v != want {
			t.Errorf("%s = %d", src, v)
		}
	}
	floats := map[string]float64{"1.5": 1.5, ".5": 0.5, "1e3": 1000, "2.5e-3": 0.0025, "1_0.2_5": 10.25}
	for src, want := range floats {
		toks, ds := Tokenize(1, []byte(src))
		if len(ds) > 0 || toks[0].Kind != token.Float {
			t.Errorf("%s: %v %v", src, toks[0], ds)
			continue
		}
		if v, ok := ParseFloat(toks[0].Text); !ok || v != want {
			t.Errorf("%s = %v", src, v)
		}
	}
	for _, bad := range []string{"1__0", "1_", "0x", "1e", "12abc", "0x_1", "0x1g", "1_e3", "1.", "1. ", "1.+2"} {
		if c := codes(bad); len(c) != 1 || c[0] != diag.MalformedNumber {
			t.Errorf("%q: %v", bad, c)
		}
	}
	if got := kinds("1.x"); !equal(got, []token.Kind{token.Int, token.Period, token.Ident, token.Semicolon, token.EOF}) {
		t.Errorf("1.x: %v", got)
	}
	// 1.e3 is left to the parser: Int '.' Ident, no lexer error.
	if c := codes("1.e3"); len(c) != 0 {
		t.Errorf("1.e3 must not error at lex time: %v", c)
	}
	if got := kinds("1.e3"); !equal(got, []token.Kind{token.Int, token.Period, token.Ident, token.Semicolon, token.EOF}) {
		t.Errorf("1.e3: %v", got)
	}
}

func TestStrings(t *testing.T) {
	cases := map[string]string{
		`"a\nb"`:     "a\nb",
		`"\t\"\\"`:   "\t\"\\",
		`"\u0416ук"`: "Жук",
		`"привет"`:   "привет",
		`""`:         "",
	}
	for src, want := range cases {
		toks, ds := Tokenize(1, []byte(src))
		if len(ds) > 0 || toks[0].Kind != token.String || toks[0].Value != want || toks[0].Text != src {
			t.Errorf("%s: %+v %v", src, toks[0], ds)
		}
	}
	bad := map[string]string{
		`"abc`:     diag.UnterminatedString,
		"\"a\nb\"": diag.NewlineInString,
		`"\q"`:     diag.InvalidEscape,
		`"\uD800"`: diag.InvalidEscape,
		`"\u12"`:   diag.InvalidEscape,
		"\"\xff\"": diag.InvalidUTF8,
	}
	for src, code := range bad {
		if c := codes(src); len(c) == 0 || c[0] != code {
			t.Errorf("%q: %v want %s", src, c, code)
		}
	}
}

func TestIllegalAndComments(t *testing.T) {
	if c := codes("x # y"); len(c) != 1 || c[0] != diag.UnexpectedChar {
		t.Errorf("#: %v", c)
	}
	if c := codes("x & y | z"); len(c) != 2 {
		t.Errorf("bitwise: %v", c)
	}
	if c := codes("'a'"); len(c) != 1 || c[0] != diag.UnexpectedChar {
		t.Errorf("char literal: %v", c)
	}
	if got := kinds("'a'"); !equal(got, []token.Kind{token.Illegal, token.EOF}) {
		t.Errorf("char literal kinds: %v", got)
	}
	if c := codes("'unterminated\n"); len(c) != 1 || c[0] != diag.UnexpectedChar {
		t.Errorf("unterminated char: %v", c)
	}
	if c := codes("ж"); len(c) != 1 || c[0] != diag.UnexpectedChar {
		t.Errorf("cyrillic ident: %v", c)
	}
	if c := codes("\xff"); len(c) != 1 || c[0] != diag.InvalidUTF8 {
		t.Errorf("utf8: %v", c)
	}
	if c := codes("x /* open"); len(c) != 1 || c[0] != diag.UnterminatedComment {
		t.Errorf("comment: %v", c)
	}
	if c := codes("x # y @@ 1__2 \"\\q\" yield"); len(c) != 4 {
		t.Errorf("all errors must be reported: %v", c)
	}
}

func TestSpansAndBOM(t *testing.T) {
	toks, _ := Tokenize(1, []byte("\xEF\xBB\xBFfoo  bar"))
	if toks[0].Text != "foo" || toks[0].Start != 3 || toks[0].End != 6 {
		t.Fatalf("foo %+v", toks[0])
	}
	if toks[1].Start != 8 || toks[1].End != 11 {
		t.Fatalf("bar %+v", toks[1])
	}
}

func FuzzTokenize(f *testing.F) {
	for _, s := range []string{
		"package p\nfunc f() { x := 1 + 2.5e3 }\n",
		"script S { @export(min=0) var A int = 0x1f }",
		"\"\\u0416\\n\" /* c */ // d",
		"#&|\xff\"",
		"1__ 0x .5 1e+",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		toks, _ := Tokenize(1, src)
		if len(toks) == 0 || toks[len(toks)-1].Kind != token.EOF {
			t.Fatal("token stream must end with EOF")
		}
		var prev uint32
		for _, tk := range toks {
			if tk.Start > tk.End || int(tk.End) > len(src) {
				t.Fatalf("bad span %+v for %d bytes", tk, len(src))
			}
			if tk.Start < prev {
				t.Fatalf("spans go backwards at %+v", tk)
			}
			prev = tk.Start
		}
	})
}
