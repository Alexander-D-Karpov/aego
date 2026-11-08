package assetfiles

import (
	"fmt"
	"image"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"aego/core/ids"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/engine/i18n"
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
	type routedFile struct {
		name    string
		handler context.ExternalFileImport
	}
	var ordinary []string
	var routed []routedFile
	for _, file := range files {
		var found bool
		for _, handler := range f.ctx.ExternalFileImports {
			for _, ext := range handler.Extensions {
				if strings.EqualFold(filepath.Ext(file), ext) {
					routed = append(routed, routedFile{file, handler})
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			ordinary = append(ordinary, file)
		}
	}
	var created []ids.AssetID
	var failures []string
	if len(ordinary) > 0 {
		ids, batch, err := f.ctx.Pipeline.ImportExternal(ordinary, dir)
		created = append(created, ids...)
		f.ctx.ApplyBatch(batch)
		if err != nil {
			failures = append(failures, err.Error())
		}
		if batch != nil {
			for id, cause := range batch.Failed {
				name, ok := f.ctx.Pipeline.PathOf(id)
				if !ok {
					name = id.String()
				}
				failures = append(failures, name+": "+cause.Error())
			}
			for _, id := range batch.Retry {
				name, ok := f.ctx.Pipeline.PathOf(id)
				if !ok {
					name = id.String()
				}
				reason := i18n.Tr("editor.assets.import_retry", "source changed during import; retry")
				if strings.EqualFold(path.Ext(name), ".aeart") {
					if data, readErr := f.ctx.Pipeline.SourceBytes(name); readErr == nil && data != nil && len(data) == 0 {
						reason = i18n.Tr("editor.assets.empty_art_import", "empty .aeart; open the copied file to initialize it, or create a new drawing")
					}
				}
				failures = append(failures, name+": "+reason)
			}
		}
	}
	for _, source := range routed {
		id, pending, err := source.handler.Import(source.name, dir)
		if err != nil {
			failures = append(failures, filepath.Base(source.name)+": "+err.Error())
		} else if pending && !id.IsZero() {
			failures = append(failures, filepath.Base(source.name)+": import returned an asset while awaiting confirmation")
		} else if pending {
			continue
		} else if id.IsZero() {
			failures = append(failures, filepath.Base(source.name)+": import did not create an asset")
		} else {
			created = append(created, id)
		}
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		return created, fmt.Errorf("%s: %s", i18n.Tr("editor.assets.import_incomplete", "Some files could not be imported; any copied files were kept"), strings.Join(failures, "; "))
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files selected for import")
	}
	return created, nil
}

func (f *Files) Reimport(id ids.AssetID) error {
	batch, err := f.ctx.Pipeline.Reimport(id)
	f.ctx.ApplyBatch(batch)
	return err
}

func (f *Files) SetStartup(id ids.AssetID, rel string) {
	assetops.SetStartup(f.ctx, id, rel)
}

func (f *Files) CreateFile(rel string, data []byte) (ids.AssetID, error) {
	return f.ops.CreateFile(f.ctx, rel, data)
}

func (f *Files) Transaction(fn func() error) error { return f.ops.Transaction(f.ctx, fn) }
