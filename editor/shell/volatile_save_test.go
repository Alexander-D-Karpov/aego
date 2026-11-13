package shell

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/document"
)

func TestVolatileSaveAllRefusesBeforeSavingOtherDocuments(t *testing.T) {
	root := t.TempDir()
	docs := document.NewManager(document.Options{Root: root})
	const file = "metadata.txt"
	before := []byte("saved metadata\n")
	if err := os.WriteFile(filepath.Join(root, file), before, 0o644); err != nil {
		t.Fatal(err)
	}
	metadata, err := docs.OpenScript(file)
	if err != nil {
		t.Fatal(err)
	}
	metadata.Code.ReplaceSnapshot([]byte("unsaved metadata\n"), false)
	if err := metadata.Code.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	if changed, err := metadata.Code.ReplaceVolatile(metadata.Code.Rev(), []byte("private edit")); err != nil || !changed {
		t.Fatal("cannot prepare volatile edit", err)
	}
	d, err := docs.NewTransientAsset(ids.NewAssetID(), "drawing", "art", asset.ArtTexture, document.View|document.EditSource)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	d.Binding.Dirty = func() bool { return true }
	d.Binding.Save = func() error { called = true; return nil }
	a := &App{ctx: &context.Editor{Docs: docs}}
	if err := a.saveAll(); err == nil || called {
		t.Fatal("shell Save All invoked a structured save despite volatile changes")
	}
	if got, err := os.ReadFile(filepath.Join(root, file)); err != nil || !bytes.Equal(got, before) {
		t.Fatal("shell Save All published text despite volatile changes", err)
	}
	if !metadata.Dirty() || !metadata.Code.VolatileDirty() {
		t.Fatal("refused shell Save All lost working changes")
	}
}

func TestVolatileSaveAllReportsStateIntroducedByLastBinding(t *testing.T) {
	root := t.TempDir()
	docs := document.NewManager(document.Options{Root: root})
	const file = "metadata.txt"
	if err := os.WriteFile(filepath.Join(root, file), []byte("saved metadata\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata, err := docs.OpenScript(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := metadata.Code.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	d, err := docs.NewTransientAsset(ids.NewAssetID(), "drawing", "art", asset.ArtTexture, document.View|document.EditSource)
	if err != nil {
		t.Fatal(err)
	}
	d.Binding.Dirty = func() bool { return true }
	d.Binding.Save = func() error {
		if changed, err := metadata.Code.ReplaceVolatile(metadata.Code.Rev(), []byte("new structured edit")); err != nil || !changed {
			t.Fatal("cannot introduce a late structured edit", err)
		}
		return nil
	}
	a := &App{ctx: &context.Editor{Docs: docs}}
	if err := a.saveAll(); err == nil || !metadata.Code.VolatileDirty() {
		t.Fatal("shell Save All reported success despite new volatile work")
	}
}
