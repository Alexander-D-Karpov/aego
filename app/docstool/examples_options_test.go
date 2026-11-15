package docstool

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestDocsBuildForwardsReviewedExampleLicenseToGendocs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the subprocess argument recorder uses the POSIX shell; Windows runtime acceptance is separate")
	}
	engine, bin := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(engine, "tools", "gendocs"), 0755); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(t.TempDir(), "arguments")
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$AEGO_DOCS_TEST_ARGUMENTS\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AEGO_DOCS_TEST_ARGUMENTS", record)
	t.Setenv("ARTSTUDIO_ARTIFACT", "")
	rootURL := "https://staging.example.invalid/docs/"
	licenseFile := filepath.Join(t.TempDir(), "example-license.txt")
	if err := os.WriteFile(licenseFile, []byte("Reviewed example terms\n"), 0644); err != nil {
		t.Fatal(err)
	}
	env := Env{Engine: engine, Stdout: io.Discard, Stderr: io.Discard,
		ExamplesLicense: "LicenseRef-Reviewed-Test-Example", ExamplesLicenseFile: licenseFile, ExamplesProduction: true,
		ExamplesURL: &rootURL, ExamplesVersion: "1.2.3",
		Native: func(args ...string) error {
			if len(args) != 4 || args[0] != "docs" || args[1] != "export" || args[2] != "--out" {
				t.Fatalf("unexpected source exporter arguments: %v", args)
			}
			return os.WriteFile(args[3], []byte(`{"version":2,"entries":[]}`), 0644)
		}}
	if err := env.Build(filepath.Join(t.TempDir(), "site"), false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	index := slices.Index(args, "-examples-license")
	if len(args) < 2 || args[0] != "run" || args[1] != "." || index < 0 || index+1 >= len(args) || args[index+1] != env.ExamplesLicense || !slices.Contains(args, "-examples-production") {
		t.Fatalf("actual gendocs subprocess lost the reviewed license policy: %v", args)
	}
	for flag, value := range map[string]string{"-examples-url": rootURL, "-examples-version": env.ExamplesVersion, "-examples-license-file": licenseFile} {
		index := slices.Index(args, flag)
		if index < 0 || index+1 >= len(args) || args[index+1] != value {
			t.Fatalf("actual gendocs subprocess lost %s: %v", flag, args)
		}
	}
}

func TestDocsBuildRejectsMissingProductionLicenseTextBeforeWriting(t *testing.T) {
	for _, file := range []string{"", filepath.Join(t.TempDir(), "missing.txt")} {
		out := filepath.Join(t.TempDir(), "site")
		env := Env{Engine: t.TempDir(), ExamplesLicense: "MIT", ExamplesLicenseFile: file, ExamplesProduction: true}
		if err := env.Build(out, false); err == nil || !strings.Contains(err.Error(), "license") {
			t.Fatalf("accepted missing reviewed license text: %v", err)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatal("missing production license text created a site")
		}
	}
}

func TestDocsBuildRejectsUnsafeOrDisabledProductionExampleRootBeforeWriting(t *testing.T) {
	for _, raw := range []string{
		"", "http://example.invalid/", "https://example.invalid/missing-slash",
		"https://example.invalid/?query=value", "https://example.invalid/#fragment", "https://user@example.invalid/",
	} {
		t.Run(raw, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "site")
			env := Env{Engine: t.TempDir(), ExamplesURL: &raw, ExamplesLicense: "LicenseRef-Reviewed-Test-Example", ExamplesProduction: true}
			if err := env.Build(out, false); err == nil || !strings.Contains(err.Error(), "examples-url") {
				t.Fatalf("accepted unsafe or disabled production examples root: %v", err)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("invalid example root created a site before preflight failed")
			}
		})
	}
}

func TestDocsBuildForwardsExplicitEmptyExampleRootForLocalPreview(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the subprocess argument recorder uses the POSIX shell; Windows runtime acceptance is separate")
	}
	engine, bin := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(engine, "tools", "gendocs"), 0755); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(t.TempDir(), "arguments")
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$AEGO_DOCS_TEST_ARGUMENTS\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AEGO_DOCS_TEST_ARGUMENTS", record)
	t.Setenv("ARTSTUDIO_ARTIFACT", "")
	empty := ""
	env := Env{Engine: engine, Stdout: io.Discard, Stderr: io.Discard, ExamplesURL: &empty,
		Native: func(args ...string) error { return os.WriteFile(args[3], []byte(`{"version":2,"entries":[]}`), 0644) }}
	if err := env.Build(filepath.Join(t.TempDir(), "site"), false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	index := slices.Index(args, "-examples-url")
	if index < 0 || index+1 >= len(args) || args[index+1] != "" {
		t.Fatalf("explicitly disabled example root was not passed to gendocs: %q", args)
	}
}

func TestDocsBuildRejectsUnreviewedProductionLicenseBeforeWriting(t *testing.T) {
	for _, license := range []string{"", " NOASSERTION ", "noassertion"} {
		t.Run(license, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "site")
			env := Env{Engine: t.TempDir(), ExamplesLicense: license, ExamplesProduction: true}
			if err := env.Build(out, false); err == nil || !strings.Contains(err.Error(), "examples-license") {
				t.Fatalf("accepted unknown production license: %v", err)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("unreviewed production request wrote output before failing")
			}
		})
	}
}
