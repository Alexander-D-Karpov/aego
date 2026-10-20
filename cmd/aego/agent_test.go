package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/agent"
)

var update = flag.Bool("update", false, "rewrite the generated command table in cli.md")

const (
	agentBegin = "<!-- agent:begin -->\n"
	agentEnd   = "<!-- agent:end -->"
)

func TestCLIReferenceListsAgentCommands(t *testing.T) {
	path := filepath.Join("..", "..", "engine", "res", "docs", "cli.md")
	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(doc)
	i, j := strings.Index(s, agentBegin), strings.Index(s, agentEnd)
	if i < 0 || j < i {
		t.Fatal("cli.md has no agent:begin/agent:end block")
	}
	var b bytes.Buffer
	agent.Markdown(&b)
	want := b.String()
	if got := s[i+len(agentBegin) : j]; got != want {
		if !*update {
			t.Fatalf("the command table in cli.md is out of date, run go test ./cmd/aego -run TestCLIReferenceListsAgentCommands -update")
		}
		s = s[:i+len(agentBegin)] + want + s[j:]
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func copyProject(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("..", "..", "examples", name)
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			switch rel {
			case ".aego", "native", "exports":
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if rel == "go.mod" || rel == "go.sum" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if rel == "project.aego" {
			data = []byte(strings.Replace(string(data), "native=true", "native=false", 1))
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestAgentSceneTreeJSON(t *testing.T) {
	dir := copyProject(t, "platformer")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	code := dispatch([]string{"scene", "tree", "scenes/main.aescene", "--json", "--project", dir})
	os.Stdout = stdout
	w.Close()
	out, _ := io.ReadAll(r)
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	var v struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(out, &v); err != nil || !v.OK {
		t.Fatalf("output %v:\n%s", err, out)
	}
}
