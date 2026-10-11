package main

import (
	"os"
	"path/filepath"
	"testing"

	"aego/demos"
)

func TestEveryDemoIsRegistered(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "demos"))
	if err != nil {
		t.Fatal(err)
	}
	registered := len(demos.Registry)
	dirs := 0
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "internal" || e.Name() == "res" {
			continue
		}
		if src, _ := filepath.Glob(filepath.Join("..", "..", "demos", e.Name(), "*.go")); len(src) == 0 {
			continue
		}
		dirs++
	}
	if registered != dirs {
		t.Fatalf("%d demo packages on disk but %d registered; add the blank import to cmd/sandbox", dirs, registered)
	}
}
