package playground

import (
	"testing"

	"aego/app/assembly/builtin"
)

func TestCheckSnippetsCompile(t *testing.T) {
	p, err := NewWith(builtin.Default())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range p.docs.All() {
		for _, s := range e.Snippets {
			if !s.Check || s.Lang != "aes" {
				continue
			}
			n++
			for _, d := range p.Compile(map[string]string{"main.aes": s.Code}) {
				if d.Severity == "error" {
					t.Errorf("%s (line %d of its page): %d:%d %s %s", e.ID, s.Line, d.Line, d.Col, d.Code, d.Message)
				}
			}
		}
	}
	if n == 0 {
		t.Fatal("no aes check blocks in the documentation")
	}
}
