package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const rootMarker = ".aego-asset-studio-root.json"

type deploymentRoot struct {
	Schema      int  `json:"schema"`
	Development bool `json:"development"`
}
type gateResult struct {
	Name      string `json:"name"`
	ExitCode  int    `json:"exitCode"`
	LogSHA256 string `json:"logSHA256"`
}
type gateReport struct {
	Schema         int          `json:"schema"`
	BuildID        string       `json:"buildID"`
	Commit         string       `json:"commit"`
	SourceSHA256   string       `json:"sourceSHA256"`
	ManifestSHA256 string       `json:"manifestSHA256"`
	Gates          []gateResult `json:"gates"`
}

var requiredGates = []string{"core", "native-build", "native-user-scenario", "file-interoperability", "remote-examples", "wasm-parity", "chromium", "firefox", "chromium-workspace-files", "firefox-workspace-files", "chromium-pending-fields", "firefox-pending-fields", "chromium-gpu", "firefox-gpu", "docs", "component-try", "budgets", "regression", "source-check", "http-bytes"}

func validateGate(b []byte, m buildManifest, hash string) error {
	var g gateReport
	if len(b) > 1<<20 {
		return errors.New("gate report too large")
	}
	if e := json.Unmarshal(b, &g); e != nil {
		return e
	}
	if g.Schema != 1 || g.BuildID != m.BuildID || g.Commit != m.Commit || g.SourceSHA256 != m.SourceSHA256 || g.ManifestSHA256 != hash {
		return errors.New("gate provenance does not match the artifact")
	}
	found := map[string]bool{}
	for _, v := range g.Gates {
		if found[v.Name] || v.ExitCode != 0 || !shaPattern.MatchString(v.LogSHA256) {
			return fmt.Errorf("invalid or failed gate %s", v.Name)
		}
		found[v.Name] = true
	}
	for _, name := range requiredGates {
		if !found[name] {
			return fmt.Errorf("required gate has not passed: %s", name)
		}
	}
	return nil
}
func initDeployment(root string, development bool) error {
	if runtime.GOOS == "windows" {
		return errors.New("atomic release deployment requires a POSIX host; standalone building is supported separately")
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || root == string(filepath.Separator) {
		return errors.New("use a new absolute, dedicated deployment directory")
	}
	if e := noSymlinkPath(filepath.Dir(root)); e != nil {
		return e
	}
	if e := os.Mkdir(root, 0755); e != nil {
		return fmt.Errorf("deployment root must not exist: %w", e)
	}
	// No recursive deletion on failure: preserve evidence and any external files.
	if e := writeNew(filepath.Join(root, rootMarker), jsonBytes(deploymentRoot{1, development})); e != nil {
		return e
	}
	for _, d := range []string{"releases", "records"} {
		mode := os.FileMode(0755)
		if d == "records" {
			mode = 0700
		}
		if e := os.Mkdir(filepath.Join(root, d), mode); e != nil {
			return e
		}
	}
	return syncDir(root)
}
func openDeployment(root string) (deploymentRoot, error) {
	var r deploymentRoot
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return r, errors.New("deployment root must be absolute and canonical")
	}
	if e := noSymlinkPath(root); e != nil {
		return r, e
	}
	b, e := readBounded(filepath.Join(root, rootMarker), 4096)
	if e != nil {
		return r, errors.New("not an initialized dedicated Asset Studio root")
	}
	if e = json.Unmarshal(b, &r); e != nil || r.Schema != 1 {
		return r, errors.New("invalid deployment marker")
	}
	for _, d := range []string{"releases", "records"} {
		if e = noSymlinkPath(filepath.Join(root, d)); e != nil {
			return r, e
		}
	}
	return r, nil
}
func lockDeployment(root string) (func(), error) {
	lock := filepath.Join(root, ".deploy-lock")
	if e := os.Mkdir(lock, 0700); e != nil {
		return nil, fmt.Errorf("deployment locked; inspect the existing lock before manual recovery: %w", e)
	}
	// Only our newly-created lock is removed. Never remove a stale lock automatically.
	return func() { _ = os.Remove(lock) }, nil
}
func syncDir(p string) error {
	f, e := os.Open(p)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func stageArtifact(root, dir, expected, gatePath string) error {
	r, e := openDeployment(root)
	if e != nil {
		return e
	}
	unlock, e := lockDeployment(root)
	if e != nil {
		return e
	}
	defer unlock()
	if !shaPattern.MatchString(expected) {
		return errors.New("a trusted manifest SHA-256 is required")
	}
	m, h, e := verify(dir, expected)
	if e != nil {
		return e
	}
	var gate []byte
	if !r.Development {
		if !m.Release {
			return errors.New("snapshot artifacts cannot be deployed to a production root")
		}
		gate, e = readBounded(gatePath, 1<<20)
		if e != nil {
			return e
		}
		if e = validateGate(gate, m, h); e != nil {
			return e
		}
	}
	final := filepath.Join(root, "releases", m.BuildID)
	if _, e = os.Lstat(final); e == nil {
		_, same, e := verify(final, h)
		if e != nil || same != h {
			return errors.New("immutable release ID already exists with different bytes")
		}
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	temp, e := os.MkdirTemp(filepath.Join(root, "releases"), ".incoming-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	paths := []string{"manifest.json"}
	for _, f := range m.Files {
		paths = append(paths, f.Path)
	}
	for _, p := range paths {
		b, e := readBounded(filepath.Join(dir, filepath.FromSlash(p)), maxFileBytes)
		if e != nil {
			return e
		}
		if e = writeNew(filepath.Join(temp, filepath.FromSlash(p)), b); e != nil {
			return e
		}
	}
	if _, _, e = verify(temp, h); e != nil {
		return e
	}
	record := filepath.Join(root, "records", m.BuildID)
	if e = os.Mkdir(record, 0750); e != nil {
		return fmt.Errorf("release record already exists or cannot be created; inspect interrupted stage: %w", e)
	}
	if len(gate) > 0 {
		if e = writeNew(filepath.Join(record, "gate.json"), gate); e != nil {
			return e
		}
	}
	if e = writeNew(filepath.Join(record, "stage.json"), jsonBytes(map[string]any{"manifestSHA256": h, "buildID": m.BuildID, "at": time.Now().UTC().Format(time.RFC3339), "development": r.Development})); e != nil {
		return e
	}
	if e = os.Chmod(temp, 0755); e != nil {
		return e
	}
	if e = os.Rename(temp, final); e != nil {
		return e
	}
	return syncDir(filepath.Join(root, "releases"))
}
func currentID(root, name string) (string, error) {
	p := filepath.Join(root, name)
	target, e := os.Readlink(p)
	if os.IsNotExist(e) {
		return "", nil
	}
	if e != nil {
		return "", e
	}
	prefix := "releases/"
	id := strings.TrimPrefix(filepath.ToSlash(target), prefix)
	if !strings.HasPrefix(filepath.ToSlash(target), prefix) || !identifier.MatchString(id) {
		return "", fmt.Errorf("unexpected %s link", name)
	}
	return id, nil
}
func replaceLink(root, name, id string) error {
	if !identifier.MatchString(id) {
		return errors.New("invalid release ID")
	}
	if _, e := currentID(root, name); e != nil {
		return e
	}
	f, e := os.CreateTemp(root, ".link-")
	if e != nil {
		return e
	}
	temp := f.Name()
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Remove(temp); e != nil {
		return e
	}
	defer os.Remove(temp)
	if e = os.Symlink(filepath.Join("releases", id), temp); e != nil {
		return e
	}
	return os.Rename(temp, filepath.Join(root, name))
}
func activateRelease(root, id, expected string, rollback bool) error {
	r, e := openDeployment(root)
	if e != nil {
		return e
	}
	unlock, e := lockDeployment(root)
	if e != nil {
		return e
	}
	defer unlock()
	if rollback {
		id, e = currentID(root, "previous")
		if e != nil {
			return e
		}
		if id == "" {
			return errors.New("no previous release")
		}
	}
	if !identifier.MatchString(id) || !shaPattern.MatchString(expected) {
		return errors.New("release ID and trusted manifest SHA-256 required")
	}
	m, h, e := verify(filepath.Join(root, "releases", id), expected)
	if e != nil {
		return e
	}
	if m.BuildID != id {
		return errors.New("release ID mismatch")
	}
	if !r.Development {
		if !m.Release {
			return errors.New("production refuses snapshots")
		}
		gate, e := readBounded(filepath.Join(root, "records", id, "gate.json"), 1<<20)
		if e != nil {
			return e
		}
		if e = validateGate(gate, m, h); e != nil {
			return e
		}
	}
	old, e := currentID(root, "current")
	if e != nil {
		return e
	}
	if old == id {
		return nil
	}
	// Open and synchronize the audit file before touching active links. A bad
	// log path must never produce a failed command after an unnoticed switch.
	logPath := filepath.Join(root, "deployments.jsonl")
	if st, e := os.Lstat(logPath); e == nil && !st.Mode().IsRegular() {
		return errors.New("deployment log is not a regular file")
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	f, e := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = f.Sync(); e != nil {
		return e
	}
	if old != "" {
		if e = replaceLink(root, "previous", old); e != nil {
			return e
		}
	}
	if e = replaceLink(root, "current", id); e != nil {
		return e
	}
	if e = syncDir(root); e != nil {
		return fmt.Errorf("current switched to %s, but directory sync failed: %w", id, e)
	}
	// The release is switched; append-only audit record includes no user content.
	row := map[string]any{"from": old, "to": id, "manifestSHA256": h, "rollback": rollback, "at": time.Now().UTC().Format(time.RFC3339)}
	b, e := json.Marshal(row)
	if e != nil {
		return e
	}
	if _, e = f.Write(append(b, '\n')); e != nil {
		return fmt.Errorf("current switched to %s, but audit append failed: %w", id, e)
	}
	if e = f.Sync(); e != nil {
		return fmt.Errorf("current switched to %s, but audit sync failed: %w", id, e)
	}
	return nil
}
func deployCLI(args []string) error {
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	root := f.String("root", "", "dedicated initialized deployment directory")
	dev := f.Bool("development", false, "init a local test root; never use for public hosting")
	dir := f.String("dir", "", "verified artifact directory")
	sha := f.String("sha256", "", "trusted manifest SHA-256 from CI")
	gate := f.String("gate", "", "CI gate report bound to this artifact")
	id := f.String("id", "", "installed immutable build ID")
	if e := f.Parse(args[1:]); e != nil {
		return e
	}
	switch args[0] {
	case "init":
		return initDeployment(*root, *dev)
	case "stage":
		return stageArtifact(*root, *dir, *sha, *gate)
	case "activate":
		return activateRelease(*root, *id, *sha, false)
	case "rollback":
		return activateRelease(*root, "", *sha, true)
	}
	return errors.New("unknown deployment operation")
}
