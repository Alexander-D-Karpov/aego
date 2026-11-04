package assetops

import (
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/project"
	"bytes"
	"errors"
	"os"
	"testing"
)

type sourceImporter struct{}

func (sourceImporter) ID() string                  { return "source" }
func (sourceImporter) Version() int                { return 1 }
func (sourceImporter) Extensions() []string        { return []string{".aeshader", ".aemat"} }
func (sourceImporter) Defaults() pipeline.Settings { return pipeline.Settings{} }
func (sourceImporter) Import(c *pipeline.ImportContext) (*pipeline.Result, error) {
	return &pipeline.Result{Kind: asset.ArtShaderInclude, Artifacts: []pipeline.Produced{{Kind: asset.ArtShaderInclude, Data: c.Source}}}, nil
}

func sourceContext(t *testing.T) *context.Editor {
	t.Helper()
	p, err := project.Create(t.TempDir(), "files", false)
	if err != nil {
		t.Fatal(err)
	}
	reg := pipeline.NewImporterRegistry()
	if err = reg.Register(sourceImporter{}); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	return &context.Editor{Project: p, Pipeline: pl, Docs: document.NewManager(document.Options{Root: p.Root})}
}

func TestCreateMaterialAndShaderIsOneUndoPreservingIDs(t *testing.T) {
	ctx := sourceContext(t)
	var o Ops
	var a, b ids.AssetID
	err := o.Transaction(ctx, func() error {
		var e error
		a, e = o.CreateFile(ctx, "assets/source.aeshader", []byte("shader"))
		if e != nil {
			return e
		}
		b, e = o.CreateFile(ctx, "assets/material.aemat", []byte("material "+a.String()))
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	ma, _ := os.ReadFile(ctx.Pipeline.Abs("assets/source.aeshader.meta"))
	mb, _ := os.ReadFile(ctx.Pipeline.Abs("assets/material.aemat.meta"))
	if err = o.Undo(ctx); err != nil {
		t.Fatal(err)
	}
	if o.CanUndo() || !o.CanRedo() || Exists(ctx, "assets/source.aeshader") || Exists(ctx, "assets/material.aemat") {
		t.Fatal("two-file creation was not one complete undo")
	}
	if err = o.Redo(ctx); err != nil {
		t.Fatal(err)
	}
	ai, _ := ctx.Pipeline.Lookup("assets/source.aeshader")
	bi, _ := ctx.Pipeline.Lookup("assets/material.aemat")
	ra, _ := os.ReadFile(ctx.Pipeline.Abs("assets/source.aeshader.meta"))
	rb, _ := os.ReadFile(ctx.Pipeline.Abs("assets/material.aemat.meta"))
	if ai != a || bi != b || !bytes.Equal(ma, ra) || !bytes.Equal(mb, rb) {
		t.Fatal("redo changed asset identity or import settings")
	}
	// Batch preflight must refuse before deleting either source.
	d, e := ctx.Docs.OpenScript("assets/source.aeshader")
	if e != nil {
		t.Fatal(e)
	}
	d.Code.Replace(0, 0, "edited ")
	if err = o.Undo(ctx); err == nil {
		t.Fatal("undo deleted unsaved source")
	}
	if !Exists(ctx, "assets/source.aeshader") || !Exists(ctx, "assets/material.aemat") || !o.CanUndo() {
		t.Fatal("rejected undo partially changed files/history")
	}
	d.Code.Undo()
	if err = o.Undo(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCreateTransactionRollsBackAndRejectsEscape(t *testing.T) {
	ctx := sourceContext(t)
	var o Ops
	rejected := errors.New("stop")
	err := o.Transaction(ctx, func() error {
		_, e := o.CreateFile(ctx, "assets/test.aeshader", []byte("one"))
		if e != nil {
			return e
		}
		return rejected
	})
	if !errors.Is(err, rejected) || Exists(ctx, "assets/test.aeshader") || Exists(ctx, "assets/test.aeshader.meta") || o.CanUndo() {
		t.Fatal("failed transaction left created files or history")
	}
	if _, err = o.CreateFile(ctx, "../escape.aeshader", []byte("no")); err == nil {
		t.Fatal("asset creation escaped project")
	}
}
