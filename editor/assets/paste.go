package assets

import (
	"image"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/context"
	"aego/platform/clip"
)

const PastedName = "Pasted"

type cutSet struct {
	list  []ids.AssetID
	paths []string
}

func report(ctx *context.Editor, err error) {
	if err != nil {
		ctx.Notify(context.LevelError, err.Error())
	}
}

func NextNumbered(ctx *context.Editor, dir, base, sep, ext string) string {
	for i := 1; ; i++ {
		if rel := path.Join(dir, base+sep+strconv.Itoa(i)+ext); !Exists(ctx, rel) {
			return rel
		}
	}
}

func EnsureDir(ctx *context.Editor, dir string) error {
	if dir == "" || dir == "." || Exists(ctx, dir) {
		return nil
	}
	if err := EnsureDir(ctx, path.Dir(dir)); err != nil {
		return err
	}
	return ctx.Pipeline.Mkdir(dir)
}

func CreatePNG(ctx *context.Editor, rel string, img image.Image) (ids.AssetID, error) {
	data := clip.EncodePNG(img)
	if len(data) == 0 {
		return ids.AssetID{}, errs.New(errs.EditorDocument, "cannot encode the image as PNG")
	}
	return CreateFile(ctx, rel, data)
}

func ProjectRel(ctx *context.Editor, abs string) (string, bool) {
	rel, err := filepath.Rel(ctx.Pipeline.Abs("."), filepath.Clean(abs))
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

func sourcePNG(ctx *context.Editor, id ids.AssetID) ([]byte, bool) {
	e, ok := ctx.Pipeline.Entry(id)
	if !ok || e.Kind != asset.ArtTexture {
		return nil, false
	}
	rel, ok := ctx.Pipeline.PathOf(id)
	if !ok {
		return nil, false
	}
	data, err := os.ReadFile(ctx.Pipeline.Abs(rel))
	if err != nil {
		return nil, false
	}
	if strings.EqualFold(path.Ext(rel), ".png") {
		return data, true
	}
	img, err := clip.DecodeImage(data)
	if err != nil {
		return nil, false
	}
	out := clip.EncodePNG(img)
	return out, len(out) > 0
}

func absPaths(ctx *context.Editor, list []ids.AssetID) []string {
	var out []string
	for _, id := range list {
		if rel, ok := ctx.Pipeline.PathOf(id); ok {
			out = append(out, filepath.Clean(ctx.Pipeline.Abs(rel)))
		}
	}
	return out
}

func CopyAssets(ctx *context.Editor, list []ids.AssetID) []string {
	if ctx.Clipboard == nil || ctx.Pipeline == nil {
		return nil
	}
	paths := absPaths(ctx, list)
	if len(paths) == 0 {
		return nil
	}
	if len(list) == 1 {
		if data, ok := sourcePNG(ctx, list[0]); ok && clip.SetImagePNG(ctx.Clipboard, data, paths) {
			return paths
		}
	}
	if !clip.SetFiles(ctx.Clipboard, paths) {
		return nil
	}
	return paths
}

func CanPaste(ctx *context.Editor) bool {
	if ctx.Clipboard == nil || ctx.Pipeline == nil {
		return false
	}
	if clip.HasFiles(ctx.Clipboard) || clip.HasImage(ctx.Clipboard) {
		return true
	}
	_, ok := clip.Files(ctx.Clipboard)
	return ok
}

func samePaths(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	norm := func(in []string) []string {
		out := make([]string, len(in))
		for i, s := range in {
			out[i] = filepath.Clean(s)
		}
		slices.Sort(out)
		return out
	}
	return slices.Equal(norm(a), norm(b))
}

func (p *Panel) selectList(list []ids.AssetID) {
	clear(p.sel)
	for _, id := range list {
		p.sel[id] = true
	}
	if n := len(list); n > 0 {
		p.active, p.anchor, p.scrollTo = list[n-1], list[0], list[0]
	}
}

func (p *Panel) CopySelected(ctx *context.Editor) error {
	p.cut = cutSet{}
	if paths := CopyAssets(ctx, p.Selection()); len(paths) > 0 {
		ctx.Notify(context.LevelInfo, "Copied "+strconv.Itoa(len(paths))+" asset(s)")
	}
	return nil
}

func (p *Panel) CutSelected(ctx *context.Editor) error {
	list := p.Selection()
	paths := CopyAssets(ctx, list)
	p.cut = cutSet{}
	if len(paths) == 0 {
		return nil
	}
	p.cut = cutSet{list: list, paths: paths}
	ctx.Notify(context.LevelInfo, "Cut "+strconv.Itoa(len(paths))+" asset(s), paste into another folder to move them")
	return nil
}

func (p *Panel) PasteHere(ctx *context.Editor) error {
	return p.Paste(ctx, p.folder)
}

func (p *Panel) Paste(ctx *context.Editor, dir string) error {
	if ctx.Clipboard == nil || ctx.Pipeline == nil {
		return nil
	}
	if files, ok := clip.Files(ctx.Clipboard); ok {
		if len(p.cut.paths) > 0 && samePaths(files, p.cut.paths) {
			return p.pasteCut(ctx, dir)
		}
		return p.pasteFiles(ctx, files, dir)
	}
	if img, ok := clip.Image(ctx.Clipboard); ok {
		return p.pasteImage(ctx, img, dir)
	}
	ctx.Notify(context.LevelInfo, "The clipboard has no image or files to paste")
	return nil
}

func (p *Panel) pasteCut(ctx *context.Editor, dir string) error {
	var items []context.AssetPayload
	for _, id := range p.cut.list {
		if e, ok := ctx.Pipeline.Entry(id); ok && !e.Missing {
			items = append(items, context.AssetPayload{ID: id, Kind: e.Kind, Name: e.Name})
		}
	}
	p.cut = cutSet{}
	if len(items) == 0 {
		return nil
	}
	p.moveInto(ctx, items, nil, dir)
	moved := make([]ids.AssetID, len(items))
	for i, it := range items {
		moved[i] = it.ID
	}
	if paths := absPaths(ctx, moved); len(paths) > 0 {
		clip.SetFiles(ctx.Clipboard, paths)
	}
	p.folder, p.search = dir, ""
	p.expanded[path.Dir(dir)] = true
	p.selectList(moved)
	return nil
}

func copyInto(ctx *context.Editor, id ids.AssetID, dir string) (ids.AssetID, error) {
	orig, ok := ctx.Pipeline.PathOf(id)
	if !ok {
		return ids.AssetID{}, errs.New(errs.EditorDocument, "the copied asset no longer exists")
	}
	dup, err := Duplicate(ctx, id)
	if err != nil || path.Dir(orig) == dir {
		return dup, err
	}
	rel, ok := ctx.Pipeline.PathOf(dup)
	if !ok {
		return dup, nil
	}
	return dup, moveFile(ctx, dup, rel, UniqueFile(ctx, dir, Stem(orig), path.Ext(orig)))
}

func (p *Panel) pasteFiles(ctx *context.Editor, files []string, dir string) error {
	var outside []string
	var made []ids.AssetID
	var first error
	keep := func(err error) {
		if err != nil && first == nil {
			first = err
		}
	}
	for _, f := range files {
		rel, inside := ProjectRel(ctx, f)
		if !inside {
			outside = append(outside, f)
			continue
		}
		id, ok := ctx.Pipeline.Lookup(rel)
		if !ok {
			keep(errs.New(errs.EditorDocument, rel+" is not an asset, folders are moved by dragging"))
			continue
		}
		dup, err := copyInto(ctx, id, dir)
		keep(err)
		if !dup.IsZero() {
			made = append(made, dup)
		}
	}
	if len(outside) > 0 {
		created, batch, err := ctx.Pipeline.ImportExternal(outside, dir)
		ctx.ApplyBatch(batch)
		keep(err)
		made = append(made, created...)
	}
	if len(made) > 0 {
		p.folder, p.search = dir, ""
		p.expanded[path.Dir(dir)] = true
		p.selectList(made)
		ctx.Notify(context.LevelInfo, "Pasted "+strconv.Itoa(len(made))+" file(s) into "+dir)
	}
	p.invalidate()
	return first
}

func (p *Panel) pasteImage(ctx *context.Editor, img image.Image, dir string) error {
	rel := NextNumbered(ctx, dir, PastedName, " ", ".png")
	if _, err := CreatePNG(ctx, rel, img); err != nil {
		return err
	}
	p.created(ctx, rel)
	p.StartRename()
	return nil
}

func (p *Panel) scrollToTile(cols int, tileH, h float32) {
	if p.scrollTo.IsZero() {
		return
	}
	k := p.viewIndex(p.scrollTo)
	p.scrollTo = ids.AssetID{}
	if k < 0 {
		return
	}
	top := float32((k+len(p.subdirs))/cols) * tileH
	if top < p.gridScroll || top+tileH > p.gridScroll+h {
		p.gridScroll = max(0, top-(h-tileH)/2)
	}
}
