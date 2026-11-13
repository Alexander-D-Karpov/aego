package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func releaseGitCommand(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// This repository is a small independent fixture, not the user's checkout.
func releaseGitFixture(t *testing.T) (string, string) {
	t.Helper()
	root := siteInputSourceFixture(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_OPTIONAL_LOCKS", "0")
	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR"} {
		if value, present := os.LookupEnv(name); present {
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Setenv(name, value) })
		}
	}
	for name, data := range map[string]string{
		".gitignore":       "*.key\n__pycache__/\n",
		"art/empty.bin":    "",
		"art/large.bin":    strings.Repeat("\x00\x91fixture\n", 8192),
		"engine/config.go": "package engine\nvar Version = \"0.12.5-dev\"\n",
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	releaseGitCommand(t, root, "init", "-q")
	releaseGitCommand(t, root, "add", "--all")
	releaseGitCommand(t, root, "-c", "user.name=Aego test", "-c", "user.email=aego-test@example.invalid", "commit", "-qm", "fixture")
	return root, releaseGitCommand(t, root, "rev-parse", "HEAD")
}

func releaseGitArtifact(t *testing.T, root, commit string, release bool) string {
	t.Helper()
	source, err := sourceHash(root)
	if err != nil {
		t.Fatal(err)
	}
	dir, manifest, _ := fixtureArtifact(t, "git-source-check")
	manifest.Release, manifest.Commit, manifest.SourceSHA256 = release, commit, source
	manifest.AppOrigin = "https://studio.invalid"
	manifest.DocsOrigins = []string{"https://docs.invalid", manifest.AppOrigin}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), jsonBytes(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func releaseIgnoredCanary(t *testing.T, root string) {
	t.Helper()
	name := "art/synthetic-untracked.key"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte("synthetic canary, not a secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if releaseGitCommand(t, root, "check-ignore", "--", name) != name || releaseGitCommand(t, root, "status", "--porcelain=v1", "--untracked-files=all") != "" {
		t.Fatal("fixture canary is not ignored by a clean Git status")
	}
}

func TestReleaseBuildRejectsIgnoredSourceInput(t *testing.T) {
	root, _ := releaseGitFixture(t)
	releaseIgnoredCanary(t, root)
	out := filepath.Join(t.TempDir(), "release")
	_, err := build(buildOptions{Root: root, Out: out, ID: "git-source-check", Origin: "https://studio.invalid", DocsOrigin: "https://docs.invalid", Release: true})
	if err == nil || !strings.Contains(err.Error(), "untracked or ignored release source") {
		t.Fatalf("ignored release input was not rejected before the build: %v", err)
	}
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		t.Fatalf("rejected build created an output: %v", err)
	}
}

func TestReleaseCheckSourceRequiresExactGitInputs(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*testing.T, string)
		want   string
	}{
		{name: "clean"},
		{name: "ignored", change: releaseIgnoredCanary, want: "untracked or ignored release source"},
		{name: "ignored bytecode", change: func(t *testing.T, root string) {
			cache := filepath.Join(root, "tools/artstudio/__pycache__")
			if err := os.MkdirAll(cache, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cache, "fixture.pyc"), []byte("synthetic bytecode"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, want: "untracked or ignored release source"},
		{name: "ignored output outside inputs", change: func(t *testing.T, root string) {
			cache := filepath.Join(root, ".cache")
			if err := os.MkdirAll(cache, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cache, "fixture.key"), []byte("synthetic output"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "skip-worktree flag", change: func(t *testing.T, root string) {
			releaseGitCommand(t, root, "update-index", "--skip-worktree", "art/input.go")
		}, want: "release source index"},
		{name: "hidden mode change", change: func(t *testing.T, root string) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows does not expose Unix executable mode bits")
			}
			releaseGitCommand(t, root, "config", "core.filemode", "false")
			if err := os.Chmod(filepath.Join(root, "art/input.go"), 0o755); err != nil {
				t.Fatal(err)
			}
			if status := releaseGitCommand(t, root, "status", "--porcelain=v1", "--untracked-files=all"); status != "" {
				t.Fatalf("fixture did not hide the mode change: %s", status)
			}
		}, want: "release source mode differs"},
		{name: "hidden tracked change", change: func(t *testing.T, root string) {
			releaseGitCommand(t, root, "update-index", "--assume-unchanged", "art/input.go")
			if err := os.WriteFile(filepath.Join(root, "art/input.go"), []byte("different source"), 0o644); err != nil {
				t.Fatal(err)
			}
			if status := releaseGitCommand(t, root, "status", "--porcelain=v1", "--untracked-files=all"); status != "" {
				t.Fatalf("fixture did not hide the tracked change: %s", status)
			}
		}, want: "release source"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, commit := releaseGitFixture(t)
			if test.change != nil {
				test.change(t, root)
			}
			dir := releaseGitArtifact(t, root, commit, true)
			err := run([]string{"check-source", "-root", root, "-dir", dir})
			if test.want == "" && err != nil || test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("check-source error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestReleaseGitBlobsCompareExactBytes(t *testing.T) {
	root, commit := releaseGitFixture(t)
	paths := []string{"art/input.go", "art/empty.bin", "art/large.bin", "art/input.go"}
	var blobs []releaseGitBlob
	for _, path := range paths {
		blobs = append(blobs, releaseGitBlob{path: path, oid: releaseGitCommand(t, root, "rev-parse", commit+":"+path), mode: "100644"})
	}
	if err := verifyReleaseBlobs(root, blobs); err != nil {
		t.Fatalf("matching Git blobs failed: %v", err)
	}
	large := filepath.Join(root, "art/large.bin")
	data, err := os.ReadFile(large)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)/2] ^= 1
	if err := os.WriteFile(large, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseBlobs(root, blobs); err == nil || !strings.Contains(err.Error(), "release source differs from Git commit") {
		t.Fatalf("same-size changed Git blob was accepted: %v", err)
	}
	if err := os.Remove(large); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseBlobs(root, blobs); err == nil {
		t.Fatal("missing tracked source was accepted")
	}
	if err := verifyReleaseBlobs(root, []releaseGitBlob{{path: "art/input.go", oid: strings.Repeat("f", 40), mode: "100644"}}); err == nil {
		t.Fatal("missing Git object was accepted")
	}
}

func TestReleaseCheckSourceRejectsAmbientGoWorkspace(t *testing.T) {
	root, commit := releaseGitFixture(t)
	dir := releaseGitArtifact(t, root, commit, true)
	work := filepath.Join(t.TempDir(), "go.work")
	if err := os.WriteFile(work, []byte("go 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOWORK", work)
	if err := run([]string{"check-source", "-root", root, "-dir", dir}); err == nil || !strings.Contains(err.Error(), "ambient Go workspace") {
		t.Fatalf("ambient workspace was not rejected: %v", err)
	}
}

func TestReleaseCheckSourceSupportsCheckoutPathWithTrailingSpace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not support checkout names ending in a space")
	}
	root, commit := releaseGitFixture(t)
	spaced := filepath.Join(t.TempDir(), "checkout with space ")
	if err := os.Rename(root, spaced); err != nil {
		t.Fatal(err)
	}
	dir := releaseGitArtifact(t, spaced, commit, true)
	if err := run([]string{"check-source", "-root", spaced, "-dir", dir}); err != nil {
		t.Fatalf("valid checkout path was rejected: %v", err)
	}
}

func TestSnapshotCheckSourceStillAllowsIgnoredWorkingInput(t *testing.T) {
	root, commit := releaseGitFixture(t)
	releaseIgnoredCanary(t, root)
	dir := releaseGitArtifact(t, root, commit, false)
	if err := run([]string{"check-source", "-root", root, "-dir", dir}); err != nil {
		t.Fatalf("snapshot source unexpectedly rejected: %v", err)
	}
}
