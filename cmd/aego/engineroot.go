package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func engineRoot(flagValue string) (string, error) {
	candidates := []string{flagValue, os.Getenv("AEGO_ROOT")}
	if _, file, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(file), "..", ".."))
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if isEngineRoot(abs) {
			return abs, nil
		}
	}
	return "", errors.New("cannot locate the aego source tree; pass --engine <dir> or set AEGO_ROOT")
}

func isEngineRoot(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "module aego" {
			return true
		}
	}
	return false
}
