package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCLIReferenceListsEveryCommand(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile(filepath.Join("..", "..", "engine", "res", "docs", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	cases := regexp.MustCompile(`case "([a-z]+)"`).FindAllStringSubmatch(string(src), -1)
	if len(cases) < 10 {
		t.Fatalf("found only %d commands in main.go", len(cases))
	}
	for _, m := range cases {
		if m[1] == "help" {
			continue
		}
		if !strings.Contains(string(doc), "\n## aego "+m[1]+"\n") {
			t.Errorf("engine/res/docs/cli.md has no section ## aego %s", m[1])
		}
	}
}
