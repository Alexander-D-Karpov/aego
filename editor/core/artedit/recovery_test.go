package artedit

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"

	"aego/art"
	"aego/asset"
	"aego/core/ids"
	"aego/editor/core/document"
)

func pendingArtRecovery(t *testing.T) (*document.Manager, *document.AssetDocument, *Document, string, []byte) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	d, err := art.New(8, 8, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	data, err := d.Encode()
	d.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "sample.aeart"), data, 0644); err != nil {
		t.Fatal(err)
	}
	id := ids.NewAssetID()
	open := func() (*document.Manager, *document.AssetDocument, *Document) {
		t.Helper()
		m := document.NewManager(document.Options{Root: root})
		if err := m.Handlers().Register(document.Handler{ID: "art", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Extensions: []string{".aeart"}, Capabilities: document.View | document.EditSource}); err != nil {
			t.Fatal(err)
		}
		a, err := m.OpenAsset(document.AssetRequest{Asset: id, Path: "assets/sample.aeart", Kind: asset.ArtTexture})
		if err != nil {
			t.Fatal(err)
		}
		b, err := Bind(m, a)
		if err != nil {
			t.Fatal(err)
		}
		return m, a, b
	}
	old, a, b := open()
	if _, err := b.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 1, Y: 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	if err := old.WriteAssetRecovery(a); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, document.RecoveryDir, "asset-"+id.String()+".recovery")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	// A second manager represents a fresh process; the first is deliberately
	// not closed as though it had terminated without cleanup.
	m, assetDoc, model := open()
	if !model.PendingRecovery {
		t.Fatal("recovery was not offered")
	}
	t.Cleanup(b.Close)
	return m, assetDoc, model, file, before
}

func TestPendingArtRecoveryCannotBeOverwrittenByNewEdits(t *testing.T) {
	m, _, b, file, before := pendingArtRecovery(t)
	defer b.Close()
	if _, err := b.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 2, Y: 2}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{G: 255, A: 255}}}}); err == nil {
		t.Fatal("editing was allowed before resolving the previous recovery")
	}
	// A direct session caller could still dirty the model; the recovery writer
	// itself must guard the old copy even when normal editor actions are blocked.
	if _, err := b.Session.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 2, Y: 2}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{G: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteAssetRecovery(b.Asset); err == nil {
		t.Fatal("a periodic recovery could overwrite the pending recovery")
	}
	if err := b.Save(); err == nil {
		t.Fatal("saving was allowed before resolving the previous recovery")
	}
	if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, before) {
		t.Fatal("the pending recovery was changed")
	}
	if err := b.Recover(); err != nil || !b.Asset.Dirty() || b.PendingRecovery {
		t.Fatalf("explicit recovery failed: %v", err)
	}
	if err := b.Save(); err != nil {
		t.Fatalf("saving the explicitly recovered drawing failed: %v", err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("explicitly saved recovery was retained: %v", err)
	}
}

func TestClosingOrDiscardingAnUnrelatedEditPreservesPendingArtRecovery(t *testing.T) {
	m, d, b, file, before := pendingArtRecovery(t)
	// Simulate an unexpected dirty session even though normal art edits are
	// blocked, then exercise the actual Discard-on-Quit cleanup path.
	if _, err := b.Session.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 2, Y: 2}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{G: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	if err := m.DiscardOpenRecovery(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, before) {
		t.Fatal("discard-on-quit erased an unresolved recovery")
	}
	if err := m.CloseAsset(d.Asset, true); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, before) {
		t.Fatal("closing the current asset erased an unresolved recovery")
	}
}

func TestExplicitDiscardOfPendingArtRecoveryAllowsEditing(t *testing.T) {
	_, _, b, file, _ := pendingArtRecovery(t)
	defer b.Close()
	if err := b.DiscardRecovery(); err != nil || b.PendingRecovery {
		t.Fatalf("explicit discard failed: %v", err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("explicitly discarded recovery still exists: %v", err)
	}
	if _, err := b.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 2, Y: 2}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{G: 255, A: 255}}}}); err != nil {
		t.Fatalf("editing stayed blocked after explicit discard: %v", err)
	}
}
