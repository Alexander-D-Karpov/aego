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
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "internal" || e.Name() == "res" {
			continue
		}
		if src, _ := filepath.Glob(filepath.Join("..", "..", "demos", e.Name(), "*.go")); len(src) == 0 {
			continue
		}
		if _, ok := demos.Registry[e.Name()]; !ok {
			t.Errorf("demo package %s registers no demo named %s; add the blank import to cmd/sandbox", e.Name(), e.Name())
		}
	}
}
