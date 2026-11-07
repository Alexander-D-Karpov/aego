package edit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aego/project"
	"aego/project/remoteexample"
)

func remoteFixture(t *testing.T) (launcherRemote, string, string, string) {
	t.Helper()
	return remoteFixtureAt(t, filepath.Join(t.TempDir(), "store"))
}

func remoteFixtureAt(t *testing.T, root string) (launcherRemote, string, string, string) {
	t.Helper()
	if _, err := remoteexample.NewStore(root, remoteexample.Limits{}); err != nil {
		t.Fatal(err)
	}
	const id = "example-000000000000000000000001"
	dir := filepath.Join(root, "projects", id)
	if _, err := project.Create(dir, "Offline example", false); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(dir, "scenes", "demo.aescene")
	if err := os.WriteFile(entry, []byte("scene version=1 {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := make([]remoteexample.File, 0, 2)
	for _, path := range []string{"project.aego", "scenes/demo.aescene"} {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(data)
		files = append(files, remoteexample.File{Path: path, Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:])})
	}
	const origin = "https://example.invalid"
	const source = origin + "/demo.json"
	archiveHash := sha256.Sum256([]byte("archive"))
	copy := remoteexample.Copy{ID: id, ManifestURL: source, Created: time.Now().UTC(), Manifest: remoteexample.Manifest{
		Format: 1, ExampleID: "example", Version: "1.0.0", Origin: origin, License: "MIT",
		Engine: remoteexample.Compatibility{Min: "0.12.1", MaxExclusive: "0.13.0"}, EntryScene: "scenes/demo.aescene", Files: files,
		Archive: remoteexample.Archive{URL: origin + "/demo.zip", Size: 7, SHA256: hex.EncodeToString(archiveHash[:])},
	}}
	data, err := json.Marshal(copy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "remote-example.receipt.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return launcherRemote{root: root}, id, source, entry
}

func TestLauncherRemoteUsesOfflineEditsAndExactConfirmation(t *testing.T) {
	host, id, source, entry := remoteFixture(t)
	ctx := context.Background()
	copies, err := host.List(ctx)
	if err != nil || len(copies) != 1 || copies[0].ID != id {
		t.Fatalf("list: %+v, %v", copies, err)
	}
	edited := []byte("scene version=1 { node \"user-edit\" {} }\n")
	if err := os.WriteFile(entry, edited, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := host.Open(ctx, source, "https://other.invalid", false); err == nil {
		t.Fatal("opening without confirmation of the exact origin succeeded")
	}
	copy, err := host.Open(ctx, source, "https://example.invalid", false)
	if err != nil || copy.ID != id || copy.Entry != entry {
		t.Fatalf("offline open: %+v, %v", copy, err)
	}
	contents, err := os.ReadFile(entry)
	if err != nil || string(contents) != string(edited) {
		t.Fatal("offline open overwrote the user's local edits")
	}
	copy, err = host.Reopen(ctx, id)
	if err != nil || copy.Entry != entry {
		t.Fatalf("reopen: %+v, %v", copy, err)
	}
	if _, err := host.Check(ctx, id, "https://other.invalid"); err == nil {
		t.Fatal("update check accepted an unapproved origin")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := host.Open(canceled, source, "https://example.invalid", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request returned %v", err)
	}
}

func TestManagedRemotePathsRetainEntryAndSafety(t *testing.T) {
	host, id, _, entry := remoteFixture(t)
	root := filepath.Dir(filepath.Dir(entry))
	copy, managed, err := host.existingProject(root)
	if err != nil || !managed || copy.ID != id || copy.Entry != entry {
		t.Fatalf("managed remote project was not recognized: %+v, %v, %v", copy, managed, err)
	}
	local := t.TempDir()
	if _, managed, err := host.existingProject(local); err != nil || managed {
		t.Fatalf("ordinary local project was treated as remote: %v, %v", managed, err)
	}
	nested := filepath.Join(root, "scenes")
	if _, managed, err := host.existingProject(nested); !managed || err == nil {
		t.Fatalf("nested project inside managed storage bypassed checks: %v, %v", managed, err)
	}
	if err := os.Remove(filepath.Join(root, "remote-example.receipt.json")); err != nil {
		t.Fatal(err)
	}
	if _, managed, err := host.existingProject(root); !managed || err == nil {
		t.Fatalf("managed copy without its receipt was treated as an ordinary local project: %v, %v", managed, err)
	}
}
