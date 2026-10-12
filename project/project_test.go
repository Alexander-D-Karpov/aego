package project

import (
	"os"
	"path/filepath"
	"testing"

	"aego/core/errs"
	"aego/core/ids"
)

func TestCreateAndLoad(t *testing.T) {
	root := t.TempDir()
	p, err := Create(root, "My Game", true)
	if err != nil {
		t.Fatal(err)
	}
	if p.Module != "my-game" {
		t.Fatalf("module = %q", p.Module)
	}
	for _, dir := range []string{p.AssetsDir(), p.ScenesDir(), p.CacheDir(), p.NativeDir()} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("missing directory %s", dir)
		}
	}
	p.MainScene = ids.NewAssetID()
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}

	back, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if back.Name != "My Game" || back.MainScene != p.MainScene || !back.Native {
		t.Fatalf("loaded = %+v", back)
	}
	if back.Window.Width != 1280 || back.Window.Title != "My Game" {
		t.Fatalf("window = %+v", back.Window)
	}
}

func TestCreateRefusesToOverwrite(t *testing.T) {
	root := t.TempDir()
	if _, err := Create(root, "A", false); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(root, "B", false); !errs.Has(err, errs.ProjectExists) {
		t.Fatalf("err = %v", err)
	}
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	if _, err := Create(root, "A", false); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "assets", "sub")
	os.MkdirAll(deep, 0o755)
	found, err := Find(deep)
	if err != nil {
		t.Fatal(err)
	}
	if found != root {
		t.Fatalf("found %q, want %q", found, root)
	}
	if _, err := Find(t.TempDir()); !errs.Has(err, errs.ProjectNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejectsNewerVersion(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, FileName), []byte("project version=99 {\n  name=\"x\"\n}\n"), 0o644)
	if _, err := Load(root); !errs.Has(err, errs.ProjectBadFile) {
		t.Fatalf("err = %v", err)
	}
}

func TestSaveIsCanonical(t *testing.T) {
	root := t.TempDir()
	p, _ := Create(root, "Game", false)
	first, _ := os.ReadFile(p.FilePath())
	back, _ := Load(root)
	back.Save()
	second, _ := os.ReadFile(p.FilePath())
	if string(first) != string(second) {
		t.Fatalf("save is not stable:\n%s\n---\n%s", first, second)
	}
}

func TestLoadKeepsLayoutEnsureDirsCreates(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("project version=1 {\n  name=\"T\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range DefaultDirs {
		if _, err := os.Stat(filepath.Join(root, d)); !os.IsNotExist(err) {
			t.Fatalf("Load created %s", d)
		}
	}
	made, err := p.EnsureDirs()
	if err != nil {
		t.Fatal(err)
	}
	if len(made) != len(DefaultDirs) {
		t.Fatalf("created %v, want %v", made, DefaultDirs)
	}
	again, err := p.EnsureDirs()
	if err != nil || len(again) != 0 {
		t.Fatalf("second EnsureDirs created %v, err %v", again, err)
	}
}
