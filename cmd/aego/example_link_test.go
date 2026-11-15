package main

import "testing"

func TestEditProtocolRejectsFlagsPathsAndExtraArguments(t *testing.T) {
	for _, args := range [][]string{
		{"--example-link"},
		{"--example-link", "--tool"},
		{"--example-link", "/tmp/project"},
		{"--example-link", "https://example.com/example.json"},
		{"--example-link", "aego://example?manifest=https%3A%2F%2Fexample.com%2Fx", "--tool", "run"},
		{"--example-link", "aego://example?manifest=https%3A%2F%2Fexample.com%2Fx&command=run"},
	} {
		if code := cmdEdit(args); code == 0 {
			t.Fatalf("unsafe protocol command accepted: %q", args)
		}
	}
}
