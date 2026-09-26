package main

import (
	"path/filepath"
	"testing"
)

func TestRepositoryRules(t *testing.T) {
	violations, err := Check(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Error(v)
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, pkg string
		want         bool
	}{
		{"core/...", "core/mathx", true},
		{"core/...", "core", true},
		{"core/...", "coreutil", false},
		{"platform", "platform/sdl", false},
		{"platform/...", "platform/sdl", true},
		{"...", "anything/at/all", true},
	}
	for _, c := range cases {
		if got := match(c.pattern, c.pkg); got != c.want {
			t.Fatalf("match(%q, %q) = %v", c.pattern, c.pkg, got)
		}
	}
}

func TestRuleSpecificity(t *testing.T) {
	r, ok := ruleFor("platform/sdl")
	if !ok || r.pkg != "platform/sdl" {
		t.Fatalf("rule = %v ok=%v", r.pkg, ok)
	}
	r, _ = ruleFor("gpu/gl33/internal/glstate")
	if r.pkg != "gpu/gl33/internal/..." {
		t.Fatalf("internal rule = %v", r.pkg)
	}
	if _, ok := ruleFor("nowhere/at/all"); ok {
		t.Fatal("unknown package matched a rule")
	}
}
