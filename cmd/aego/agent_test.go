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
	"aego/engine/i18n"
)

var update = flag.Bool("update", false, "rewrite the generated command table in cli.md")

const (
	agentBegin = "<!-- agent:begin -->\n"
	agentEnd   = "<!-- agent:end -->"
)

func TestCLIReferenceListsAgentCommands(t *testing.T) {
	var en bytes.Buffer
	agent.Markdown(&en)
	checkTable(t, "cli.md", en.String())
	b := i18n.Default()
	prev := b.Language()
	b.SetLanguage("ru")
	defer b.SetLanguage(prev)
	var ru bytes.Buffer
	agent.MarkdownIn(&ru, "Команда", "Значение", func(c *agent.Command) string { return i18n.Lookup(agent.DocKey(c), c.Doc) })
	checkTable(t, "cli.ru.md", ru.String())
}

// checkTable compares the generated command table of a CLI reference page, or rewrites it with -update.
func checkTable(t *testing.T, name, want string) {
	t.Helper()
	path := filepath.Join("..", "..", "engine", "res", "docs", name)
	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(doc)
	i, j := strings.Index(s, agentBegin), strings.Index(s, agentEnd)
	if i < 0 || j < i {
		t.Fatal(name + " has no agent:begin/agent:end block")
	}
	if got := s[i+len(agentBegin) : j]; got != want {
		if !*update {
			t.Fatalf("the command table in %s is out of date, run go test ./cmd/aego -run TestCLIReferenceListsAgentCommands -update", name)
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

func captureStdout(t *testing.T, run func() int) (int, []byte) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	stdout := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = stdout
		w.Close()
	}()
	type result struct {
		out []byte
		err error
	}
	read := make(chan result, 1)
	go func() {
		out, err := io.ReadAll(r)
		read <- result{out, err}
	}()
	code := run()
	os.Stdout = stdout
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got := <-read
	if got.err != nil {
		t.Fatal(got.err)
	}
	return code, got.out
}

func TestCaptureStdoutLargerThanPipe(t *testing.T) {
	want := bytes.Repeat([]byte("large output\n"), 1<<17)
	code, got := captureStdout(t, func() int {
		if _, err := os.Stdout.Write(want); err != nil {
			t.Error(err)
			return 1
		}
		return 0
	})
	if code != 0 || !bytes.Equal(got, want) {
		t.Fatalf("captured %d of %d bytes (exit %d)", len(got), len(want), code)
	}
}

func TestAgentSceneTreeJSON(t *testing.T) {
	dir := copyProject(t, "platformer")
	code, out := captureStdout(t, func() int {
		return dispatch([]string{"scene", "tree", "scenes/main.aescene", "--json", "--project", dir})
	})
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

// The top-down example needs no native module, so the CLI can run it from a copy.
func TestAgentSceneRunCounters(t *testing.T) {
	dir := copyProject(t, "topdown")
	code, out := captureStdout(t, func() int {
		return dispatch([]string{"scene", "run", "scenes/main.aescene", "--frames", "300", "--json", "--project", dir})
	})
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	var v struct {
		Value struct {
			Counters map[string]uint64 `json:"counters"`
		} `json:"value"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("output %v:\n%s", err, out)
	}
	c := v.Value.Counters
	if c["gameplay.steps"] == 0 || c["camera.outputs"] != 1 {
		t.Fatalf("counters %v, want gameplay steps and one camera output", c)
	}
}
