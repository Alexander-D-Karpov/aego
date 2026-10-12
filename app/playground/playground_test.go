package playground

import (
	"strings"
	"testing"
)

func open(t *testing.T) *Playground {
	t.Helper()
	p, err := New()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExamplesRun(t *testing.T) {
	p := open(t)
	ex := p.Examples()
	if len(ex) == 0 {
		t.Fatal("the documentation has no run blocks")
	}
	for _, e := range ex {
		t.Run(e.ID, func(t *testing.T) {
			res := p.Run(map[string]string{"main.aes": e.Code}, DefaultFrames)
			for _, d := range res.Diagnostics {
				if d.Severity == "error" {
					t.Errorf("%s:%d:%d %s %s", d.File, d.Line, d.Col, d.Code, d.Message)
				}
			}
			for _, f := range res.Faults {
				t.Errorf("%s faulted with %s at %s:%d: %s", f.Script, f.Code, f.File, f.Line, f.Message)
			}
			if res.Frames != DefaultFrames {
				t.Errorf("ran %d frames, want %d", res.Frames, DefaultFrames)
			}
		})
	}
}

func TestBlinkLogs(t *testing.T) {
	p := open(t)
	code := ""
	for _, e := range p.Examples() {
		if strings.HasPrefix(e.ID, "language/coroutines/") {
			code = e.Code
			break
		}
	}
	if code == "" {
		t.Fatal("language/coroutines has no run block")
	}
	res := p.Run(map[string]string{"main.aes": code}, 60)
	var got []string
	for _, l := range res.Log {
		if l.Channel == "script" {
			got = append(got, l.Text)
		}
	}
	want := []string{"blink 0", "blink 1", "blink 2", "done"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestCompileReportsPosition(t *testing.T) {
	p := open(t)
	src := "package demo\n\nscript S {\n\tfunc Update(dt float) {\n\t\tx := missing\n\t}\n}\n"
	ds := p.Compile(map[string]string{"main.aes": src})
	for _, d := range ds {
		if d.Severity == "error" && d.File == "demo/main.aes" && d.Line == 5 {
			return
		}
	}
	t.Fatalf("no error on demo/main.aes:5, got %+v", ds)
}

func TestTokensUseUTF16Offsets(t *testing.T) {
	toks := Tokens("// привет\nvar x int")
	if len(toks) == 0 || toks[0].Kind != "comment" || toks[0].End != 9 {
		t.Fatalf("comment token = %+v", toks)
	}
	for _, tk := range toks {
		if tk.Kind == "keyword" {
			if tk.Start != 10 || tk.End != 13 {
				t.Fatalf("var token = %+v", tk)
			}
			return
		}
	}
	t.Fatal("no keyword token")
}

func TestReply(t *testing.T) {
	p := open(t)
	if out := p.Reply("tokens", `{"source":"var"}`); !strings.Contains(out, `"keyword"`) {
		t.Fatalf("tokens reply = %s", out)
	}
	if out := p.Reply("nope", ""); !strings.Contains(out, `"error"`) {
		t.Fatalf("unknown method reply = %s", out)
	}
	if out := p.Reply("stubs", ""); !strings.Contains(out, "stub:aego/math.aes") {
		t.Fatalf("stubs reply has no aego/math")
	}
}
