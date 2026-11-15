package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func requirePOSIXDeployment(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX release deployment; not a Windows authoring test")
	}
}
func developmentRoot(t *testing.T) string {
	requirePOSIXDeployment(t)
	t.Helper()
	root := filepath.Join(physicalTempDir(t), "asset-studio")
	if e := initDeployment(root, true); e != nil {
		t.Fatal(e)
	}
	return root
}
func TestAtomicPromotionRollbackAndImmutableVersions(t *testing.T) {
	root := developmentRoot(t)
	a, _, ha := fixtureArtifact(t, "first")
	b, _, hb := fixtureArtifact(t, "second")
	if e := stageArtifact(root, a, ha, ""); e != nil {
		t.Fatal(e)
	}
	if e := activateRelease(root, "first", ha, false); e != nil {
		t.Fatal(e)
	}
	if e := stageArtifact(root, b, hb, ""); e != nil {
		t.Fatal(e)
	}
	if e := activateRelease(root, "second", hb, false); e != nil {
		t.Fatal(e)
	}
	if v, _ := currentID(root, "current"); v != "second" {
		t.Fatal(v)
	}
	if v, _ := currentID(root, "previous"); v != "first" {
		t.Fatal(v)
	}
	if e := activateRelease(root, "", ha, true); e != nil {
		t.Fatal(e)
	}
	if v, _ := currentID(root, "current"); v != "first" {
		t.Fatal(v)
	}
	if e := activateRelease(root, "second", hb, false); e != nil {
		t.Fatal(e)
	}
	if _, _, e := verify(filepath.Join(root, "releases", "first"), ha); e != nil {
		t.Fatal("previous release changed", e)
	}
	if e := stageArtifact(root, b, hb, ""); e != nil {
		t.Fatal("idempotent stage", e)
	}
}
func TestProductionRejectsSnapshotAndMissingGates(t *testing.T) {
	requirePOSIXDeployment(t)
	root := filepath.Join(physicalTempDir(t), "production")
	if e := initDeployment(root, false); e != nil {
		t.Fatal(e)
	}
	dir, m, h := fixtureArtifact(t, "release")
	if e := stageArtifact(root, dir, h, ""); e == nil || e.Error() != "snapshot artifacts cannot be deployed to a production root" {
		t.Fatalf("snapshot refusal = %v", e)
	}
	m.Release = true
	m.Commit = strings.Repeat("a", 40)
	m.AppOrigin = "https://aeditor.akarpov.ru"
	m.DocsOrigins = []string{"https://aego.akarpov.ru"}
	if e := os.WriteFile(filepath.Join(dir, "manifest.json"), jsonBytes(m), 0644); e != nil {
		t.Fatal(e)
	}
	h = digest(jsonBytes(m))
	if e := stageArtifact(root, dir, h, ""); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("missing gate refusal = %v", e)
	}
	g := gateReport{Schema: 1, BuildID: m.BuildID, Commit: m.Commit, SourceSHA256: m.SourceSHA256, ManifestSHA256: h}
	for _, name := range requiredGates {
		g.Gates = append(g.Gates, gateResult{name, 0, digest([]byte(name))})
	}
	if e := validateGate(jsonBytes(g), m, h); e != nil {
		t.Fatal(e)
	}
	for missing := range g.Gates {
		t.Run("missing_"+g.Gates[missing].Name, func(t *testing.T) {
			partial := g
			partial.Gates = append([]gateResult(nil), g.Gates[:missing]...)
			partial.Gates = append(partial.Gates, g.Gates[missing+1:]...)
			if e := validateGate(jsonBytes(partial), m, h); e == nil {
				t.Fatal("partial CI accepted")
			}
		})
	}
}
func TestDeploymentNeverTouchesSiblingAndRefusesSharedRoot(t *testing.T) {
	requirePOSIXDeployment(t)
	parent := physicalTempDir(t)
	sibling := filepath.Join(parent, "docs.txt")
	_ = os.WriteFile(sibling, []byte("keep"), 0644)
	if e := initDeployment(parent, true); e == nil {
		t.Fatal("used existing shared root")
	}
	root := filepath.Join(parent, "asset-studio")
	if e := initDeployment(root, true); e != nil {
		t.Fatal(e)
	}
	a, _, h := fixtureArtifact(t, "one")
	if e := stageArtifact(root, a, h, ""); e != nil {
		t.Fatal(e)
	}
	if e := activateRelease(root, "one", h, false); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(sibling)
	if string(b) != "keep" {
		t.Fatal("modified neighbour")
	}
}
func TestDeploymentLocksSymlinksAndWrongHashes(t *testing.T) {
	root := developmentRoot(t)
	dir, _, h := fixtureArtifact(t, "one")
	if e := stageArtifact(root, dir, strings.Repeat("0", 64), ""); e == nil || e.Error() != "manifest SHA-256 mismatch" {
		t.Fatalf("bad SHA refusal = %v", e)
	}
	unlock, e := lockDeployment(root)
	if e != nil {
		t.Fatal(e)
	}
	defer unlock()
	if e = stageArtifact(root, dir, h, ""); e == nil || !strings.HasPrefix(e.Error(), "deployment locked;") {
		t.Fatalf("concurrent stage refusal = %v", e)
	}
	link := filepath.Join(physicalTempDir(t), "root-link")
	fixtureSymlink(t, root, link)
	if _, e = openDeployment(link); e == nil {
		t.Fatal("symlink root accepted")
	} else if want := "symlink path refused: " + link; e.Error() != want {
		t.Fatalf("root symlink error = %v; want %s", e, want)
	}
}

func TestDeploymentRejectsSymlinkAncestors(t *testing.T) {
	requirePOSIXDeployment(t)
	parent := physicalTempDir(t)
	real := filepath.Join(parent, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "link")
	fixtureSymlink(t, real, link)
	want := "symlink path refused: " + link
	if err := initDeployment(filepath.Join(link, "new-root"), true); err == nil || err.Error() != want {
		t.Fatalf("initialization through symlink = %v; want %s", err, want)
	}
	if _, err := os.Lstat(filepath.Join(real, "new-root")); !os.IsNotExist(err) {
		t.Fatalf("initialization touched symlink target: %v", err)
	}
	if err := initDeployment(filepath.Join(real, "existing-root"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := openDeployment(filepath.Join(link, "existing-root")); err == nil || err.Error() != want {
		t.Fatalf("open through symlink = %v; want %s", err, want)
	}
}

func TestBadAuditPathDoesNotSwitchCurrent(t *testing.T) {
	root := developmentRoot(t)
	a, _, ha := fixtureArtifact(t, "first")
	b, _, hb := fixtureArtifact(t, "second")
	if err := stageArtifact(root, a, ha, ""); err != nil {
		t.Fatal(err)
	}
	if err := stageArtifact(root, b, hb, ""); err != nil {
		t.Fatal(err)
	}
	if err := activateRelease(root, "first", ha, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "deployments.jsonl")); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(physicalTempDir(t), "unrelated")
	if err := os.WriteFile(target, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "deployments.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := activateRelease(root, "second", hb, false); err == nil {
		t.Fatal("symlink audit accepted")
	}
	if id, _ := currentID(root, "current"); id != "first" {
		t.Fatal("current changed", id)
	}
	bytes, _ := os.ReadFile(target)
	if string(bytes) != "keep" {
		t.Fatal("external log target modified")
	}
}
