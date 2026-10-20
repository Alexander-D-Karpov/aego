package diag

import (
	"strings"
	"testing"
)

func TestEveryCodeHasExample(t *testing.T) {
	for _, c := range Codes() {
		ex, ok := Example(c)
		switch {
		case !ok:
			t.Errorf("%s has no example", c)
		case ex.Why != "":
			if ex.Bad != "" || ex.Good != "" {
				t.Errorf("%s is marked not reproducible but has sources", c)
			}
		case ex.Bad == "" || ex.Good == "" || ex.Bad == ex.Good && len(ex.Files) == 0:
			t.Errorf("%s needs a broken and a different fixed version", c)
		}
		if Phase(c) == "Other" {
			t.Errorf("%s has no phase", c)
		}
	}
	for c := range examples {
		if _, ok := Explain(c); !ok {
			t.Errorf("example for unknown code %s", c)
		}
	}
}

func TestExampleSources(t *testing.T) {
	ex, _ := Example(NotExported)
	bad, good := ex.Sources(false), ex.Sources(true)
	if len(bad) != 2 || !strings.Contains(bad["rules/rules.aes"], "startLives") || !strings.Contains(good["rules/rules.aes"], "StartLives") {
		t.Fatalf("sources %v / %v", bad, good)
	}
	if ex, _ := Example(PackageName); ex.MainFile() != "level/main.aes" {
		t.Fatalf("main file %s", ex.MainFile())
	}
}
