package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	p := filepath.Join(t.TempDir(), "out.txt")
	data := "goos: linux\npkg: aego/x\n" +
		"BenchmarkStep-16 \t 100 \t 1000 ns/op \t 0 B/op \t 0 allocs/op\n" +
		"BenchmarkStep-16 \t 100 \t 1100 ns/op \t 0 B/op \t 0 allocs/op\n" +
		"BenchmarkRay \t 50 \t 300 ns/op \t 2 allocs/op\n" +
		"PASS\n"
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parse(p)
	if err != nil {
		t.Fatal(err)
	}
	if s := got["aego/x.BenchmarkStep"]; s == nil || len(s.ns) != 2 || s.ns[1] != 1100 || len(s.allocs) != 2 {
		t.Fatalf("step = %+v", s)
	}
	if s := got["aego/x.BenchmarkRay"]; s == nil || s.allocs[0] != 2 {
		t.Fatalf("ray = %+v", s)
	}
}
