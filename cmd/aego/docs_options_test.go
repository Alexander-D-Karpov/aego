package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDocsCLIRecognizesExampleLicenseAndProductionFlags(t *testing.T) {
	out := filepath.Join(t.TempDir(), "site")
	// A valid engine path and a deliberately unknown publication license reach
	// Env.Build's fail-closed policy (exit1), not an unknown CLI flag (exit2).
	if code := cmdDocs([]string{"build", "--engine", "../..", "--out", out, "--playground=false", "--examples-license", "NOASSERTION", "--examples-production"}); code != 1 {
		t.Fatalf("license flags did not reach the publication policy: exit %d", code)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("rejected production command created a site")
	}
}

func TestDocsCLIRecognizesExampleOriginAndVersionFlags(t *testing.T) {
	for _, raw := range []string{"", "http://example.invalid/"} {
		out := filepath.Join(t.TempDir(), "site")
		code := cmdDocs([]string{"build", "--engine", "../..", "--out", out, "--playground=false", "--examples-license", "LicenseRef-Reviewed-Test-Example", "--examples-production", "--examples-url=" + raw, "--examples-version", "1.2.3"})
		if code != 1 {
			t.Fatalf("origin/version flags did not reach production preflight: exit %d", code)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("rejected production origin created a site")
		}
	}
}

func TestDocsCLIRejectsMissingProductionLicenseFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "site")
	missing := filepath.Join(t.TempDir(), "missing-license.txt")
	code := cmdDocs([]string{"build", "--engine", "../..", "--out", out, "--playground=false", "--examples-license", "MIT", "--examples-license-file", missing, "--examples-production"})
	if code != 1 {
		t.Fatalf("missing license file did not fail the production preflight: exit %d", code)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("missing license file created a site")
	}
}
