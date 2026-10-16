package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnknownBlocksSurvive(t *testing.T) {
	dir := t.TempDir()
	src := "project version=1 {\n  name=\"g\"\n  module=\"g\"\n  custom {\n    a=1\n    nested {\n      b=[1, 2]\n    }\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	first := p.Encode()
	if !strings.Contains(string(first), "  custom {\n    a=1\n    nested {\n      b=[1, 2]\n    }\n  }\n") {
		t.Fatalf("custom block changed:\n%s", first)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(again.Encode()) != string(first) {
		t.Fatal("second round trip differs")
	}
}

func TestReplaceBlockKeepsPosition(t *testing.T) {
	p := &Project{}
	p.SetBlock(nodeOf("a"))
	p.SetBlock(nodeOf("old"))
	p.SetBlock(nodeOf("c"))
	p.ReplaceBlock(nodeOf("new"), "new", "old")
	got := ""
	for _, b := range p.Extra {
		got += b.Head + " "
	}
	if got != "a new c " {
		t.Fatalf("got %q", got)
	}
}
