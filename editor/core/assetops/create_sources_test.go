package assetops

import (
	"bytes"
	"os"
	"testing"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/editor/core/context"
	"aego/editor/core/document"
)

func pairSources(t *testing.T, ctx *context.Editor, second []byte) []pipeline.SourceEdit {
	t.Helper()
	var edits []pipeline.SourceEdit
	for _, item := range []struct {
		path string
		body []byte
	}{{"assets/image.aeshader", []byte("image")}, {"assets/effect.aemat", second}} {
		meta, err := ctx.Pipeline.NewSourceMeta(item.path)
		if err != nil {
			t.Fatal(err)
		}
		edits = append(edits, pipeline.SourceEdit{Path: item.path, After: item.body}, pipeline.SourceEdit{Path: item.path + pipeline.MetaSuffix, After: pipeline.WriteMeta(meta)})
	}
	return edits
}

func TestCreateSourcesAtomicPairHistoryAndDirtyBinaryGuard(t *testing.T) {
	ctx := sourceContext(t)
	edits := pairSources(t, ctx, []byte("effect"))
	var ops Ops
	created, err := ops.CreateSources(ctx, edits)
	if err != nil || len(created) != 2 {
		t.Fatalf("creation: %v, %v", created, err)
	}
	original := map[string][]byte{}
	for _, edit := range edits {
		body, err := ctx.Pipeline.SourceBytes(edit.Path)
		if err != nil || body == nil {
			t.Fatalf("missing pair member %s: %v", edit.Path, err)
		}
		original[edit.Path] = body
	}
	// Keep caller-owned buffers out of retained history.
	edits[0].After[0] = 'X'
	doc, err := ctx.Docs.AcquireAsset(document.AssetRequest{Asset: created[0], Path: "assets/image.aeshader", Kind: asset.ArtShaderInclude})
	if err != nil {
		t.Fatal(err)
	}
	dirty := true
	doc.Binding.Dirty = func() bool { return dirty }
	if err := ops.Undo(ctx); err == nil || !ops.CanUndo() || ops.CanRedo() {
		t.Fatal("creation undo accepted dirty structured pixels or lost its entry")
	}
	for path, want := range original {
		got, _ := ctx.Pipeline.SourceBytes(path)
		if !bytes.Equal(got, want) {
			t.Fatalf("dirty guard partially deleted %s", path)
		}
	}
	dirty = false
	// A recovery awaiting an explicit user decision is not represented by
	// Dirty(), but removing its source would still destroy a valid recovery path.
	doc.Binding.PendingRecovery = func() bool { return true }
	if err := ops.Undo(ctx); err == nil || !ops.CanUndo() || ops.CanRedo() {
		t.Fatal("creation undo accepted an unresolved recovery")
	}
	for path, want := range original {
		got, err := ctx.Pipeline.SourceBytes(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("pending recovery guard changed %s: %v", path, err)
		}
	}
	doc.Binding.PendingRecovery = nil
	if err := ops.Undo(ctx); err != nil {
		t.Fatal(err)
	}
	if ops.CanUndo() || !ops.CanRedo() || ctx.Docs.AssetDocument(created[0]) != nil {
		t.Fatal("pair did not have one complete undo including its clean view")
	}
	for path := range original {
		if got, err := ctx.Pipeline.SourceBytes(path); err != nil || got != nil {
			t.Fatalf("undo left %s: %v", path, err)
		}
	}
	if err := ops.Redo(ctx); err != nil {
		t.Fatal(err)
	}
	for path, want := range original {
		got, _ := ctx.Pipeline.SourceBytes(path)
		if !bytes.Equal(got, want) {
			t.Fatalf("redo changed source or metadata %s", path)
		}
	}
	for i, path := range []string{"assets/image.aeshader", "assets/effect.aemat"} {
		if id, ok := ctx.Pipeline.Lookup(path); !ok || id != created[i] {
			t.Fatal("redo changed a stable asset identity")
		}
	}
}

func TestCreateSourcesConflictPreflightAndImportRollback(t *testing.T) {
	t.Run("import failure", func(t *testing.T) {
		ctx := sourceContext(t)
		edits := pairSources(t, ctx, []byte("reject"))
		var ops Ops
		if _, err := ops.CreateSources(ctx, edits); err == nil {
			t.Fatal("invalid second source was accepted")
		}
		for _, edit := range edits {
			if got, err := ctx.Pipeline.SourceBytes(edit.Path); err != nil || got != nil {
				t.Fatalf("failed batch left %s: %v", edit.Path, err)
			}
		}
		if ops.CanUndo() || ops.CanRedo() {
			t.Fatal("rolled-back creation produced a history entry")
		}
	})
	t.Run("external change guards whole group", func(t *testing.T) {
		ctx := sourceContext(t)
		edits := pairSources(t, ctx, []byte("effect"))
		var ops Ops
		if _, err := ops.CreateSources(ctx, edits); err != nil {
			t.Fatal(err)
		}
		const external = "externally edited"
		if err := os.WriteFile(ctx.Pipeline.Abs(edits[2].Path), []byte(external), 0644); err != nil {
			t.Fatal(err)
		}
		if err := ops.Undo(ctx); err == nil {
			t.Fatal("undo overwrote an external writer")
		}
		if !Exists(ctx, edits[0].Path) || !Exists(ctx, edits[1].Path) || !ops.CanUndo() {
			t.Fatal("external conflict partially applied the group")
		}
		got, _ := ctx.Pipeline.SourceBytes(edits[2].Path)
		if string(got) != external {
			t.Fatal("external source changed")
		}
	})
	t.Run("existing sidecar blocks all creation", func(t *testing.T) {
		ctx := sourceContext(t)
		edits := pairSources(t, ctx, []byte("effect"))
		if err := os.WriteFile(ctx.Pipeline.Abs(edits[3].Path), edits[3].After, 0644); err != nil {
			t.Fatal(err)
		}
		var ops Ops
		if _, err := ops.CreateSources(ctx, edits); err == nil || Exists(ctx, edits[0].Path) || Exists(ctx, edits[2].Path) || ops.CanUndo() {
			t.Fatal("existing metadata was not rejected before writes")
		}
	})
}
