package assetfiles

import (
	"image"

	"aego/core/ids"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
)

type Files struct {
	ctx *context.Editor
	ops assetops.Ops
}

func New(ctx *context.Editor) *Files {
	return &Files{ctx: ctx}
}

func (f *Files) History() *assetops.Ops {
	return &f.ops
}

func (f *Files) Clipboard() context.DataClipboard {
	return f.ctx.Clipboard
}

func (f *Files) ScriptEditor() bool {
	return f.ctx.Code != nil || f.ctx.OpenScript != nil
}

func (f *Files) CanNewScene() bool {
	return f.ctx.NewScene != nil
}

func (f *Files) CanNewPrefab() bool {
	return f.ctx.NewPrefab != nil
}

func (f *Files) NewScene(rel string) error {
	return f.ctx.NewScene(rel)
}

func (f *Files) NewPrefab(rel string) error {
	return f.ctx.NewPrefab(rel)
}

func (f *Files) Apply(list []assetops.FileOp) error {
	return f.ops.Apply(f.ctx, list)
}

func (f *Files) Undo() error {
	return f.ops.Undo(f.ctx)
}

func (f *Files) Redo() error {
	return f.ops.Redo(f.ctx)
}

func (f *Files) CanUndo() bool {
	return f.ops.CanUndo()
}

func (f *Files) CanRedo() bool {
	return f.ops.CanRedo()
}

func (f *Files) EnsureDir(dir string) error {
	return assetops.EnsureDir(f.ctx, dir)
}

func (f *Files) Exists(rel string) bool {
	return assetops.Exists(f.ctx, rel)
}

func (f *Files) UniqueFile(dir, stem, ext string) string {
	return assetops.UniqueFile(f.ctx, dir, stem, ext)
}

func (f *Files) NextNumbered(dir, base, sep, ext string) string {
	return assetops.NextNumbered(f.ctx, dir, base, sep, ext)
}

func (f *Files) Delete(list []ids.AssetID) error {
	return assetops.Delete(f.ctx, list)
}

func (f *Files) DeleteDir(dir string) error {
	return assetops.DeleteDir(f.ctx, dir)
}

func (f *Files) Duplicate(id ids.AssetID) (ids.AssetID, error) {
	return assetops.Duplicate(f.ctx, id)
}

func (f *Files) MoveFile(id ids.AssetID, from, to string) error {
	return assetops.MoveFile(f.ctx, id, from, to)
}

func (f *Files) CreateScript(dir, name string) (string, error) {
	return assetops.CreateScript(f.ctx, dir, name)
}

func (f *Files) CreatePNG(rel string, img image.Image) (ids.AssetID, error) {
	return assetops.CreatePNG(f.ctx, rel, img)
}

func (f *Files) CopyAssets(list []ids.AssetID) []string {
	return assetops.CopyAssets(f.ctx, list)
}

func (f *Files) AbsPaths(list []ids.AssetID) []string {
	return assetops.AbsPaths(f.ctx, list)
}

func (f *Files) CanPaste() bool {
	return assetops.CanPaste(f.ctx)
}

func (f *Files) PackageOf(dir string) string {
	return assetops.PackageOf(f.ctx, dir)
}

func (f *Files) ImportExternal(files []string, dir string) ([]ids.AssetID, error) {
	created, batch, err := f.ctx.Pipeline.ImportExternal(files, dir)
	f.ctx.ApplyBatch(batch)
	return created, err
}

func (f *Files) Reimport(id ids.AssetID) error {
	batch, err := f.ctx.Pipeline.Reimport(id)
	f.ctx.ApplyBatch(batch)
	return err
}

func (f *Files) SetStartup(id ids.AssetID, rel string) {
	assetops.SetStartup(f.ctx, id, rel)
}
