package remoteexample

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPackageUsesExistingHTTPSCloneAndOfflineStore(t *testing.T) {
	f := newFixture(t)
	files := map[string][]byte{
		"project.aego":           []byte("project version=1 { name=\"Complete\" module=\"game\" native=false }\n"),
		"scenes/main.aescene":    []byte("scene version=1 {}\n"),
		"scripts/demo.aes":       []byte("package demo\n"),
		"assets/sprite.png":      {137, 80, 78, 71},
		"assets/sprite.png.meta": []byte("meta version=1 { id=\"10000000000000000000000000000001\" importer=\"texture\" }\n"),
	}
	m := f.m
	m.AssetRefs = []string{"10000000000000000000000000000001"}
	m, manifest, archive, err := BuildPackage(m, files, f.url(), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	_, manifest2, archive2, err := BuildPackage(m, files, f.url(), Limits{})
	if err != nil || !bytes.Equal(manifest, manifest2) || !bytes.Equal(archive, archive2) {
		t.Fatal("identical sources did not produce identical package bytes", err)
	}
	parsed, err := ParseManifest(manifest, f.url(), Limits{})
	if err != nil || parsed.Archive.SHA256 != hashBytes(archive) || len(parsed.AssetRefs) != 1 {
		t.Fatal("distribution manifest does not describe archive/ref bytes", err)
	}
	z, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil || len(z.File) != len(files) {
		t.Fatal("bad source ZIP", err)
	}
	for _, file := range z.File {
		r, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil || !bytes.Equal(b, files[file.Name]) {
			t.Fatalf("source bytes changed: %s, %v", file.Name, err)
		}
	}
	f.mu.Lock()
	f.m, f.packed = m, archive
	f.mu.Unlock()
	copy := f.clone(t)
	for name, want := range files {
		b, err := os.ReadFile(filepath.Join(copy.Directory, name))
		if err != nil || !bytes.Equal(b, want) {
			t.Fatalf("complete source was lost in HTTPS install: %s: %v", name, err)
		}
	}
	user := []byte("scene version=1 { node \"123\" name=\"My edit\" {} }\n")
	if err := os.WriteFile(copy.Entry, user, 0600); err != nil {
		t.Fatal(err)
	}
	f.server.Close()
	local, err := f.store.OpenRecent(f.url())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(local.Entry)
	if err != nil || !bytes.Equal(got, user) {
		t.Fatal("offline open lost edited source", err)
	}
	if _, err := f.client.CheckUpdate(context.Background(), local, f.server.URL); err == nil {
		t.Fatal("offline update check unexpectedly succeeded")
	}
	got, _ = os.ReadFile(local.Entry)
	if !bytes.Equal(got, user) {
		t.Fatal("failed update changed local work")
	}
}

func TestBuildPackageRejectsIncompleteOrUnsafeSources(t *testing.T) {
	base := map[string][]byte{"project.aego": []byte("project {}"), "scenes/main.aescene": []byte("scene {}")}
	m := exampleManifest("https://examples.test", []byte("placeholder"), base)
	for _, name := range []string{"../outside.txt", "/absolute.txt", "assets/CLOCK$.txt", "assets/secret.env", ".git/config", "engine/main.go", "native/addon.dll", "assets/scene.aescene:ads", "remote-example.receipt.json"} {
		t.Run(name, func(t *testing.T) {
			files := map[string][]byte{}
			for p, data := range base {
				files[p] = data
			}
			files[name] = []byte("bad")
			if _, _, _, err := BuildPackage(m, files, "https://examples.test/manifest.json", Limits{}); err == nil {
				t.Fatal("unsafe package accepted")
			}
		})
	}
	for _, change := range []func(*Manifest){
		func(m *Manifest) { m.EntryScene = "scenes/missing.aescene" },
		func(m *Manifest) { m.Archive.URL = "https://elsewhere.test/project.zip" },
		func(m *Manifest) { m.AssetRefs = []string{"bad"} },
		func(m *Manifest) {
			m.AssetRefs = []string{"10000000000000000000000000000001", "10000000000000000000000000000001"}
		},
	} {
		copy := m
		change(&copy)
		if _, _, _, err := BuildPackage(copy, base, "https://examples.test/manifest.json", Limits{}); err == nil {
			t.Fatal("invalid package metadata accepted")
		}
	}
	if _, _, _, err := BuildPackage(m, base, "https://examples.test/manifest.json", Limits{ArchiveBytes: 16}); err == nil {
		t.Fatal("archive byte budget ignored")
	}
}
