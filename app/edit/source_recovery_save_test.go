package edit

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/art"
	"aego/editor/core/artedit"
	"aego/scene"
)

// The real headless shell owns the document manager and pipeline. Simulate a
// pending, incomplete source journal in this test-owned project; opening a
// project with a pre-existing journal is a separate explicit-recovery path.
func TestOpenDocumentsCannotSaveAcrossPendingSourceRecovery(t *testing.T) {
	t.Setenv("SDL_AUDIO_DRIVER", "dummy")
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	p := newProject(t)
	const artPath = "assets/recovery-guard.aeart"
	const scriptPath = "scripts/recovery-guard.aes"
	const scenePath = "scenes/recovery-guard.aescene"
	drawing, err := art.New(8, 8, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	artBefore, err := drawing.Encode()
	drawing.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, artPath), artBefore, 0o644); err != nil {
		t.Fatal(err)
	}
	scriptBefore := []byte("package game\n\nscript GuardFixture {}\n")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(p.Root, scriptPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, scriptPath), scriptBefore, 0o644); err != nil {
		t.Fatal(err)
	}
	writeScene(t, p, scenePath, func(sc *scene.Scene) { sc.Create(sc.Root(), "Keep") })
	sceneBefore, err := os.ReadFile(filepath.Join(p.Root, scenePath))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, headless())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Step() {
		t.Fatal("editor stopped during startup")
	}
	ed := s.App.Context()
	id, ok := ed.Pipeline.Lookup(artPath)
	if !ok {
		t.Fatal("drawing was not imported")
	}
	doc, err := ed.OpenAsset(id)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok || doc.Metadata == nil {
		t.Fatal("the canonical art document was not bound")
	}
	metaBefore, err := os.ReadFile(filepath.Join(p.Root, artPath+".meta"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = model.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 2, Y: 2}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ed.Docs.WriteAssetRecovery(doc); err != nil {
		t.Fatal(err)
	}
	recovery := filepath.Join(p.Root, ".aego", "recovery", "asset-"+id.String()+".recovery")
	recoveryBefore, err := os.ReadFile(recovery)
	if err != nil {
		t.Fatal(err)
	}
	script, err := ed.Docs.OpenScript(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	script.Code.ReplaceSnapshot(append(bytes.Clone(scriptBefore), []byte("// unsaved edit\n")...), false)
	doc.Metadata.Code.ReplaceSnapshot(append(bytes.Clone(metaBefore), []byte("\n// unsaved sidecar edit\n")...), false)
	sceneDoc, err := ed.Docs.Open(scenePath)
	if err != nil {
		t.Fatal(err)
	}
	sceneDoc.MarkDirty()
	cursor := model.History.Cursor()

	journalDir := filepath.Join(p.Root, ".aego", "source-transactions", "active")
	if err := os.MkdirAll(journalDir, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(journalDir, "preserve-incomplete-journal")
	if err := os.WriteFile(marker, []byte("do not remove"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, save := range map[string]func() error{
		"direct art binding": model.Save,
		"asset command":      func() error { return ed.SaveAsset(doc) },
		"metadata sidecar":   func() error { return ed.Docs.SaveScript(doc.Metadata) },
		"text document":      func() error { return ed.Docs.SaveScript(script) },
		"scene document":     func() error { return ed.Docs.Save(sceneDoc.ID) },
	} {
		if err := save(); err == nil || !strings.Contains(err.Error(), "source transaction") {
			t.Errorf("%s saved through pending source recovery: %v", name, err)
		}
	}
	if !doc.Dirty() || !doc.Metadata.Dirty() || !script.Dirty() || !sceneDoc.Dirty() || model.History.Cursor() != cursor {
		t.Fatal("rejected saves cleared dirty state or changed history")
	}
	preview, _, err := model.Capture(model.Session.Frame, art.Albedo)
	if err != nil || preview.NRGBAAt(2, 2).R != 255 || preview.NRGBAAt(2, 2).A != 255 {
		t.Fatal("rejected save lost unsaved art pixels", err)
	}
	for _, check := range []struct {
		path string
		want []byte
	}{
		{filepath.Join(p.Root, artPath), artBefore},
		{filepath.Join(p.Root, artPath+".meta"), metaBefore},
		{filepath.Join(p.Root, scriptPath), scriptBefore},
		{filepath.Join(p.Root, scenePath), sceneBefore},
		{recovery, recoveryBefore},
		{marker, []byte("do not remove")},
	} {
		data, err := os.ReadFile(check.path)
		if err != nil || !bytes.Equal(data, check.want) {
			t.Errorf("rejected save changed %s: %v", check.path, err)
		}
	}
}
