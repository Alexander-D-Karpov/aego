package agent

import (
	"bytes"
	"encoding/json"
	"testing"
)

func mustFind(t *testing.T, name string) *Command {
	t.Helper()
	c, ok := Find(name)
	if !ok {
		t.Fatalf("no command %s", name)
	}
	return c
}

func TestParseArgs(t *testing.T) {
	set := mustFind(t, "scene.set")
	a, f, err := parseArgs(set, []string{"s.aescene", "--json", "Player", "--dry-run", "Body2D.mass", "-3"})
	if err != nil || !f.json || a.Str("value") != "-3" || a.Str("node") != "Player" || !a.Bool("dry-run") {
		t.Fatalf("flags after positionals: %v %+v %v", a, f, err)
	}
	add := mustFind(t, "scene.add-node")
	for _, toks := range [][]string{{"s", "N", "--index=1"}, {"s", "N", "--index", "1"}} {
		a, _, err := parseArgs(add, toks)
		if n, _ := a.Int("index", -1); err != nil || n != 1 {
			t.Fatalf("%v: %v %v", toks, a, err)
		}
	}
	a, _, err = parseArgs(set, []string{"s", "--", "Player", "Body2D.mass", "--odd"})
	if err != nil || a.Str("value") != "--odd" {
		t.Fatalf("after --: %v %v", a, err)
	}
	if _, _, err := parseArgs(set, []string{"s", "--nope"}); err == nil {
		t.Fatal("an unknown flag was accepted")
	}
	if _, _, err := parseArgs(set, []string{"s", "a", "b", "c", "d"}); err == nil {
		t.Fatal("an extra positional was accepted")
	}
	var out, errw bytes.Buffer
	if code := Main(nil, []string{"scene", "set", "s", "--nope"}, &out, &errw, Options{}); code != 2 {
		t.Fatalf("unknown flag exit %d", code)
	}
	if code := Main(nil, []string{"scene", "set", "a", "b", "c", "d", "e"}, &out, &errw, Options{}); code != 2 {
		t.Fatalf("extra positional exit %d", code)
	}
	if code := Main(nil, []string{"scene", "help"}, &out, &errw, Options{}); code != 0 {
		t.Fatalf("help exit %d", code)
	}
}

func TestSchemasAndToolNames(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Commands() {
		if seen[c.Tool()] {
			t.Fatalf("tool name %s is used twice", c.Tool())
		}
		seen[c.Tool()] = true
		b, err := json.Marshal(c.Schema())
		if err != nil {
			t.Fatal(err)
		}
		var sch struct {
			Type       string                    `json:"type"`
			Properties map[string]map[string]any `json:"properties"`
			Required   []string                  `json:"required"`
		}
		if err := json.Unmarshal(b, &sch); err != nil || sch.Type != "object" {
			t.Fatalf("%s: %s", c.Name, b)
		}
		var want []string
		for _, x := range c.AllArgs() {
			if x.Required {
				want = append(want, x.Name)
			}
			if _, ok := sch.Properties[x.Name]; !ok {
				t.Fatalf("%s: no property %s", c.Name, x.Name)
			}
		}
		if len(want) != len(sch.Required) {
			t.Fatalf("%s: required %v, want %v", c.Name, sch.Required, want)
		}
	}
}
