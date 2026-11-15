package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func releaseFixtureWrite(t *testing.T, root, name, content string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStudioSourceCoverageUsesDirectoryBoundaries(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"art", true}, {"art/internal/aseprite", true}, {"modules/render2d", true},
		{"modules/render2d/editor", true}, {"tools/artstudio/wasm", true},
		{".", false}, {"audio", false}, {"art-extra", false},
		{"modules/render2d-extra", false}, {"../art", false},
	} {
		if got := studioSourceCoversDir(filepath.FromSlash(tc.path)); got != tc.want {
			t.Errorf("coverage for %q = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestStudioGoArgsKeepSnapshotAndReleaseModesSeparate(t *testing.T) {
	for _, subcommand := range []string{"list", "build"} {
		release := studioGoArgs(true, subcommand, "-buildvcs=false", "./example")
		if !slices.Equal(release, []string{"go", subcommand, "-mod=readonly", "-buildvcs=false", "./example"}) {
			t.Fatalf("release %s arguments = %v", subcommand, release)
		}
		snapshot := studioGoArgs(false, subcommand, "-buildvcs=false", "./example")
		if !slices.Equal(snapshot, []string{"go", subcommand, "-buildvcs=false", "./example"}) {
			t.Fatalf("snapshot %s arguments changed: %v", subcommand, snapshot)
		}
	}
}

// These are independent miniature repositories, never the user's checkout.
func TestReleaseCheckSourceRejectsUncoveredLocalDependencies(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*testing.T, string)
		error string
	}{
		{name: "project-package", setup: func(t *testing.T, root string) {
			releaseFixtureWrite(t, root, "audio/uncovered/probe.go", "package uncovered\nimport \"embed\"\n//go:embed res/*\nvar Files embed.FS\n")
			releaseFixtureWrite(t, root, "audio/uncovered/res/keep.txt", "public fixture")
			releaseFixtureWrite(t, root, "tools/artstudio/previewwasm/main.go", "package main\nimport _ \"aego/audio/uncovered\"\nfunc main() {}\n")
		}, error: "outside release source coverage"},
		{name: "external-local-replace", setup: func(t *testing.T, root string) {
			external := t.TempDir()
			releaseFixtureWrite(t, external, "go.mod", "module example.invalid/local\n\ngo 1.27\n")
			releaseFixtureWrite(t, external, "local.go", "package local\n")
			mod := "module aego\n\ngo 1.27\n\nrequire example.invalid/local v0.0.0\nreplace example.invalid/local => " + strconv.Quote(filepath.ToSlash(external)) + "\n"
			releaseFixtureWrite(t, root, "go.mod", mod)
			releaseFixtureWrite(t, root, "tools/artstudio/previewwasm/main.go", "package main\nimport _ \"example.invalid/local\"\nfunc main() {}\n")
		}, error: "outside release source coverage"},
		{name: "uncovered-symlink-to-covered", setup: func(t *testing.T, root string) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows symlink creation may require extra privileges")
			}
			releaseFixtureWrite(t, root, "art/fixturepkg/probe.go", "package fixturepkg\n")
			if err := os.MkdirAll(filepath.Join(root, "audio"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("../art/fixturepkg", filepath.Join(root, "audio/uncovered")); err != nil {
				t.Fatal(err)
			}
			releaseFixtureWrite(t, root, "tools/artstudio/previewwasm/main.go", "package main\nimport _ \"aego/audio/uncovered\"\nfunc main() {}\n")
		}, error: "outside release source coverage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, _ := releaseGitFixture(t)
			tc.setup(t, root)
			releaseGitCommand(t, root, "add", "--all")
			releaseGitCommand(t, root, "-c", "user.name=Aego test", "-c", "user.email=aego-test@example.invalid", "commit", "-qm", "dependency fixture")
			commit := releaseGitCommand(t, root, "rev-parse", "HEAD")
			if tc.name == "project-package" {
				releaseFixtureWrite(t, root, "audio/uncovered/res/synthetic.key", "synthetic canary, not a secret")
				if got := releaseGitCommand(t, root, "check-ignore", "--", "audio/uncovered/res/synthetic.key"); got != "audio/uncovered/res/synthetic.key" {
					t.Fatal("fixture canary was not ignored")
				}
			}
			if status := releaseGitCommand(t, root, "status", "--porcelain=v1", "--untracked-files=all"); status != "" {
				t.Fatalf("fixture Git status is not clean: %s", status)
			}
			dir := releaseGitArtifact(t, root, commit, true)
			if err := run([]string{"check-source", "-root", root, "-dir", dir}); err == nil || !strings.Contains(err.Error(), tc.error) {
				t.Fatalf("release accepted uncovered dependency: %v", err)
			}
			snapshot := releaseGitArtifact(t, root, commit, false)
			if err := run([]string{"check-source", "-root", root, "-dir", snapshot}); err != nil {
				t.Fatalf("snapshot behavior changed: %v", err)
			}
		})
	}
}

func TestReleaseCheckSourceUsesReadonlyModuleMode(t *testing.T) {
	root, _ := releaseGitFixture(t)
	// An ambient vendor selection must not change the graph being checked.
	// The unused dependency intentionally has no matching vendor entry.
	releaseFixtureWrite(t, root, "go.mod", "module aego\n\ngo 1.27\n\nrequire example.invalid/unused v1.0.0\n")
	releaseFixtureWrite(t, root, "vendor/modules.txt", "# synthetic incomplete vendor inventory\n")
	releaseGitCommand(t, root, "add", "--all")
	releaseGitCommand(t, root, "-c", "user.name=Aego test", "-c", "user.email=aego-test@example.invalid", "commit", "-qm", "vendor fixture")
	commit := releaseGitCommand(t, root, "rev-parse", "HEAD")
	dir := releaseGitArtifact(t, root, commit, true)
	before, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOFLAGS", "-mod=vendor")
	if err := run([]string{"check-source", "-root", root, "-dir", dir}); err != nil {
		t.Fatalf("ambient vendor mode changed the release graph: %v", err)
	}
	if after, err := os.ReadFile(filepath.Join(root, "go.mod")); err != nil || string(after) != string(before) {
		t.Fatal("dependency inspection changed go.mod", err)
	}
	if status := releaseGitCommand(t, root, "status", "--porcelain=v1", "--untracked-files=all"); status != "" {
		t.Fatalf("dependency inspection changed the fixture: %s", status)
	}
}
