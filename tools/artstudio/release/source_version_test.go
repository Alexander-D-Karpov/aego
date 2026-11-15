package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceEngineVersionReadsPackagedTree(t *testing.T) {
	for _, test := range []struct {
		name, declaration, expected string
	}{
		{"development", `var Version = "0.12.5-dev"`, "0.12.5-dev"},
		{"release", `var Version = "0.12.5"`, "0.12.5"},
		{"constant", `const Version = "0.12.6+build.3"`, "0.12.6+build.3"},
		{"group", "var (Other = \"ignored\"; Version string = \"0.12.7-rc.1\")", "0.12.7-rc.1"},
		{"multiple names", `var Other, Version = "ignored", "0.12.8"`, "0.12.8"},
		{"comment ignored", "// var Version = \"9.9.9\"\nvar Version = \"0.12.5-dev\"", "0.12.5-dev"},
		{"missing", `var Other = "0.12.5"`, ""},
		{"expression", `var Version = "0.12." + "5"`, ""},
		{"uninitialized", "var Version string", ""},
		{"number", "var Version = 12", ""},
		{"malformed", "var Version =", ""},
		{"duplicate", "var Version = \"0.12.5\"\nvar Version = \"0.12.6\"", ""},
		{"invalid", `var Version = "not a version"`, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "engine"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "engine", "config.go"), []byte("package engine\n"+test.declaration+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			actual, err := sourceEngineVersion(root)
			if test.expected == "" {
				if err == nil {
					t.Fatalf("accepted ambiguous or unavailable source version: %q", actual)
				}
			} else if err != nil || actual != test.expected {
				t.Fatalf("source version = %q, %v; expected %q", actual, err, test.expected)
			}
		})
	}
}

func TestSourceEngineVersionDoesNotInventMissingFile(t *testing.T) {
	if _, err := sourceEngineVersion(t.TempDir()); err == nil {
		t.Fatal("missing source was replaced by a builder/default version")
	}
}
