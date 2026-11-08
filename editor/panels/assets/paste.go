package assets

import (
	"image"
	"path"
	"strconv"

	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/platform/clip"
)

type cutSet struct {
	list  []ids.AssetID
	paths []string
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

func (p *Panel) CopySelected() error {
	p.cut = cutSet{}
	if paths := p.d.Files.CopyAssets(p.fileSelection()); len(paths) > 0 {
		p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.assets.copied_paths_asset_s", "Copied {paths} asset(s)", i18n.Str("paths", strconv.Itoa(len(paths)))))
	}
	return nil
}

func (p *Panel) CutSelected() error {
	list := p.fileSelection()
	paths := p.d.Files.CopyAssets(list)
	p.cut = cutSet{}
	if len(paths) == 0 {
		return nil
	}
	p.cut = cutSet{list: list, paths: paths}
	p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.assets.cut_paths_asset_s_paste", "Cut {paths} asset(s), paste into another folder to move them", i18n.Str("paths", strconv.Itoa(len(paths)))))
	return nil
}

func (p *Panel) PasteHere() error {
	return p.Paste(p.folder)
}

func (p *Panel) Paste(dir string) error {
	if p.d.Files.Clipboard() == nil || p.d.Pipeline == nil {
		return nil
	}
	if files, ok := clip.Files(p.d.Files.Clipboard()); ok {
		if len(p.cut.paths) > 0 && assetops.SamePaths(files, p.cut.paths) {
			return p.pasteCut(dir)
		}
		return p.pasteFiles(files, dir)
	}
	if img, ok := clip.Image(p.d.Files.Clipboard()); ok {
		return p.pasteImage(img, dir)
	}
	p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.assets.the_clipboard_has_no_image", "The clipboard has no image or files to paste"))
	return nil
}

func (p *Panel) pasteCut(dir string) error {
	var items []context.AssetPayload
	for _, id := range p.cut.list {
		if e, ok := p.d.Pipeline.Entry(id); ok && !e.Missing {
			items = append(items, context.AssetPayload{ID: id, Kind: e.Kind, Name: e.Name})
		}
	}
	p.cut = cutSet{}
	if len(items) == 0 {
		return nil
	}
	p.moveInto(items, nil, dir)
	moved := make([]ids.AssetID, len(items))
	for i, it := range items {
		moved[i] = it.ID
	}
	if paths := p.d.Files.AbsPaths(moved); len(paths) > 0 {
		clip.SetFiles(p.d.Files.Clipboard(), paths)
	}
	p.folder, p.search = dir, ""
	p.expanded[path.Dir(dir)] = true
	p.selectList(moved)
	return nil
}

func (p *Panel) copyInto(id ids.AssetID, dir string) (ids.AssetID, error) {
	orig, ok := p.d.Pipeline.PathOf(id)
	if !ok {
		return ids.AssetID{}, errs.New(errs.EditorDocument, "the copied asset no longer exists")
	}
	dup, err := p.d.Files.Duplicate(id)
	if err != nil || path.Dir(orig) == dir {
		return dup, err
	}
	rel, ok := p.d.Pipeline.PathOf(dup)
	if !ok {
		return dup, nil
	}
	return dup, p.d.Files.MoveFile(dup, rel, p.d.Files.UniqueFile(dir, assetops.Stem(orig), path.Ext(orig)))
}

func (p *Panel) pasteFiles(files []string, dir string) error {
	var outside []string
	var made []ids.AssetID
	var first error
	keep := func(err error) {
		if err != nil && first == nil {
			first = err
		}
	}
	for _, f := range files {
		rel, inside := assetops.RelIn(p.d.Pipeline, f)
		if !inside {
			outside = append(outside, f)
			continue
		}
		id, ok := p.d.Pipeline.Lookup(rel)
		if !ok {
			keep(errs.New(errs.EditorDocument, rel+" is not an asset, folders are moved by dragging"))
			continue
		}
		dup, err := p.copyInto(id, dir)
		keep(err)
		if !dup.IsZero() {
			made = append(made, dup)
		}
	}
	if len(outside) > 0 {
		created, err := p.d.Files.ImportExternal(outside, dir)
		keep(err)
		made = append(made, created...)
	}
	if len(made) > 0 {
		p.folder, p.search = dir, ""
		p.expanded[path.Dir(dir)] = true
		p.selectList(made)
		if first == nil {
			p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.assets.pasted_made_file_s_into", "Pasted {made} file(s) into {dir}", i18n.Str("made", strconv.Itoa(len(made))), i18n.Str("dir", dir)))
		}
	}
	p.invalidate()
	return first
}

func (p *Panel) pasteImage(img image.Image, dir string) error {
	rel := p.d.Files.NextNumbered(dir, assetops.PastedName, " ", ".png")
	if _, err := p.d.Files.CreatePNG(rel, img); err != nil {
		return err
	}
	p.created(rel)
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
