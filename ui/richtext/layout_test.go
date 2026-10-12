package richtext

import "testing"

type mono7 struct{}

func (mono7) Width(f Face, s string) float32 {
	n := float32(0)
	for range s {
		n++
	}
	if f == H1 {
		return n * 12
	}
	return n * 7
}

func (mono7) Height(f Face) float32 {
	if f == H1 {
		return 24
	}
	return 16
}

func TestWrap(t *testing.T) {
	lines := wrap(Inline("aaa bbb `ccc` ddd"), Body, 0, 7*8, mono7{})
	if len(lines) != 2 {
		t.Fatalf("%d lines: %+v", len(lines), lines)
	}
	if lines[0][0].Text != "aaa bbb" || lines[1][0].Text != "ccc" || lines[1][0].Face != Mono || lines[1][1].Text != " ddd" {
		t.Fatalf("%+v", lines)
	}
	if lines[1][1].X != 21 {
		t.Fatalf("x = %v", lines[1][1].X)
	}
}

func TestLayout(t *testing.T) {
	md := "# Coroutines\n\nA coroutine pauses with [yield](ref:keyword/yield).\n\n- one\n- two\n\n| Form | Resumes |\n|---|---|\n| `yield` | on the next frame of the game |\n\n```aes run\nyield wait(0.1)\n\treturn\n```\n"
	hl := func(lang, line string) []Token {
		if lang == "aes" && len(line) >= 5 && line[:5] == "yield" {
			return []Token{{Start: 0, End: 5, Kind: "keyword"}}
		}
		return nil
	}
	d := Layout(Parse(md), 200, mono7{}, hl)
	var kinds []LineKind
	link, code, run, header := false, false, false, false
	for _, l := range d.Lines {
		kinds = append(kinds, l.Kind)
		for _, r := range l.Runs {
			if r.Target == "ref:keyword/yield" && r.Text == "yield" {
				link = true
			}
			if r.Token == "keyword" && r.Text == "yield" {
				code = true
			}
		}
		run = run || l.Run
		header = header || l.Header
	}
	if !link || !code || !run || !header {
		t.Fatalf("link %v code %v run %v header %v", link, code, run, header)
	}
	if kinds[0] != HeadingLine || d.Lines[0].H != 24 {
		t.Fatalf("heading line %+v", d.Lines[0])
	}
	for i := 1; i < len(d.Lines); i++ {
		if d.Lines[i].Y < d.Lines[i-1].Y+d.Lines[i-1].H {
			t.Fatalf("line %d overlaps the previous one", i)
		}
	}
	for _, l := range d.Lines {
		for _, r := range l.Runs {
			if l.Kind != CodeLine && r.X+r.W > 200+0.01 && len(l.Runs) > 1 {
				t.Fatalf("run %q overflows: %v+%v", r.Text, r.X, r.W)
			}
		}
	}
}
