package playground

import (
	"regexp"
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
		files := map[string]string{}
		for _, s := range e.Snippets {
			if !s.Check || s.Lang != "aes" {
				continue
			}
			n++
			name := "main"
			if m := scriptDecl.FindStringSubmatch(s.Code); m != nil {
				name = m[1] + "/" + m[2]
			}
			files[name] = s.Code
			for _, d := range p.Compile(files) {
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

// scriptDecl names a block by package and script, so a later block of a page replaces an earlier version of the same script.
var scriptDecl = regexp.MustCompile(`(?m)^package (\w+)[\s\S]*?^script (\w+)`)
