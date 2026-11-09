package main

import "testing"

func TestStandaloneDependencyAllowlist(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"bytes", true},
		{"example.com/library", true},
		{"aego/art", true},
		{"aego/art/internal/aseprite", true},
		{"aego/tools/artstudio/host", true},
		{"aego/tools/artstudio/wasm", true},
		{"aego/art/internal", false},
		{"aego/art/internal/other", false},
		{"aego/art/internal/aseprite/other", false},
		{"aego/art/other", false},
		{"aego/tools/artstudio/host/other", false},
		{"aego/engine", false},
		{"aego/editor", false},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := standaloneDependencyAllowed(tc.path); got != tc.want {
				t.Fatalf("standaloneDependencyAllowed(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
