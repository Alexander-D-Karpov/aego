package assets

import (
	"path"
	"strconv"
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/editor/context"
	"aego/editor/document"
	"aego/ui/widgets"
)

func (p *Panel) menus(ctx *context.Editor) {
	ui := ctx.UI
	if widgets.ContextMenu(ui, p.assetMenuID) {
		p.assetItems(ctx)
		widgets.EndContextMenu(ui)
	}
	if widgets.ContextMenu(ui, p.dirMenuID) {
		p.dirItems(ctx, p.menuDir)
		widgets.EndContextMenu(ui)
	}
}

func copyText(ctx *context.Editor, s string) {
	if ctx.Clipboard != nil {
		ctx.Clipboard.SetText(s)
	}
}

func (p *Panel) assetItems(ctx *context.Editor) {
	ui := ctx.UI
	a := p.find(p.active)
	if a == nil || !p.sel[a.ID] {
		return
	}
	single := len(p.sel) == 1
	reg := ctx.Commands
	if widgets.MenuItem(ui, "Open", "", single && openable(a.Kind), false) {
		p.open(ctx, a)
	}
	widgets.Separator(ui)
	if widgets.MenuItem(ui, "Copy", reg.Shortcut(CommandCopy), true, false) {
		report(ctx, p.CopySelected(ctx))
	}
	if widgets.MenuItem(ui, "Cut", reg.Shortcut(CommandCut), true, false) {
		report(ctx, p.CutSelected(ctx))
	}
	if widgets.MenuItem(ui, "Paste", reg.Shortcut(CommandPaste), CanPaste(ctx), false) {
		report(ctx, p.Paste(ctx, path.Dir(a.Path)))
	}
	widgets.Separator(ui)
	if widgets.MenuItem(ui, "Rename", reg.Shortcut(CommandRename), single, false) {
		p.StartRename()
	}
	if widgets.MenuItem(ui, "Duplicate", reg.Shortcut(CommandDuplicate), true, false) {
		report(ctx, p.DuplicateSelected(ctx))
	}
	if widgets.MenuItem(ui, "Delete", reg.Shortcut(CommandDelete), true, false) {
		p.DeleteSelected(ctx)
	}
	if widgets.MenuItem(ui, "Reimport", "", true, false) {
		report(ctx, p.ReimportSelected(ctx))
	}
	if single {
		id, rel, name := a.ID, a.Path, a.Name
		switch a.Kind {
		case asset.ArtScene:
			widgets.Separator(ui)
			startup := ctx.Project != nil && ctx.Project.MainScene == id
			if widgets.MenuItem(ui, "Set as Startup Scene", "", !startup, startup) {
				SetStartup(ctx, id, rel)
			}
		case asset.ArtPrefab:
			widgets.Separator(ui)
			if widgets.MenuItem(ui, "Instantiate in Scene", "", ctx.Doc() != nil && p.Instantiate != nil, false) {
				p.Instantiate(id, path.Base(name))
			}
		case asset.ArtScript:
			widgets.Separator(ui)
			nodes := ctx.Selected()
			if widgets.MenuItem(ui, "Attach to Selected", "", len(nodes) > 0, false) {
				AttachScriptAsset(ctx, nodes, id)
			}
		}
	}
	widgets.Separator(ui)
	if widgets.MenuItem(ui, "Reveal in File Manager", "", single, false) {
		report(ctx, reveal(ctx.Pipeline.Abs(a.Path), false))
	}
	if widgets.MenuItem(ui, "Copy Path", "", true, false) {
		var paths []string
		for _, id := range p.Selection() {
			if rel, ok := ctx.Pipeline.PathOf(id); ok {
				paths = append(paths, rel)
			}
		}
		copyText(ctx, strings.Join(paths, "\n"))
	}
}

func (p *Panel) dirItems(ctx *context.Editor, dir string) {
	ui := ctx.UI
	reg := ctx.Commands
	if widgets.SubMenu(ui, "New", 200) {
		if widgets.MenuItem(ui, "Folder", "", true, false) {
			p.newFolder(ctx, dir)
		}
		if widgets.MenuItem(ui, "Scene…", "", ctx.NewScene != nil, false) {
			p.newDoc(ctx, dir, "New scene", document.SceneExt, ctx.NewScene)
		}
		if widgets.MenuItem(ui, "Prefab…", "", ctx.NewPrefab != nil, false) {
			p.newDoc(ctx, dir, "New prefab", PrefabExt, ctx.NewPrefab)
		}
		widgets.Separator(ui)
		if widgets.MenuItem(ui, "Script…", "", true, false) {
			p.newScript(ctx, dir)
		}
		if widgets.MenuItem(ui, "Script Package…", "", true, false) {
			p.newPackage(ctx, dir)
		}
		widgets.EndSubMenu(ui)
	}
	if widgets.MenuItem(ui, "Paste", reg.Shortcut(CommandPaste), CanPaste(ctx), false) {
		report(ctx, p.Paste(ctx, dir))
	}
	widgets.Separator(ui)
	top := path.Dir(dir) == "."
	if widgets.MenuItem(ui, "Rename", "", !top, false) {
		p.expanded[path.Dir(dir)] = true
		p.renamingDir, p.renameDirStart = dir, true
	}
	if widgets.MenuItem(ui, "Delete…", "", !top, false) {
		p.deleteDir(ctx, dir)
	}
	widgets.Separator(ui)
	if widgets.MenuItem(ui, "Reveal in File Manager", "", true, false) {
		report(ctx, reveal(ctx.Pipeline.Abs(dir), true))
	}
	if widgets.MenuItem(ui, "Copy Path", "", true, false) {
		copyText(ctx, dir)
	}
}

func (p *Panel) created(ctx *context.Editor, rel string) {
	p.invalidate()
	p.refresh(ctx)
	p.folder = path.Dir(rel)
	p.search = ""
	p.expanded[path.Dir(p.folder)] = true
	if id, ok := ctx.Pipeline.Lookup(rel); ok {
		p.selectOnly(id)
		p.scrollTo = id
	}
}

func (p *Panel) newDoc(ctx *context.Editor, dir, title, ext string, create func(string) error) {
	ctx.Ask(context.Question{
		Title:   title,
		Fields:  []string{"Name"},
		Values:  []string{Stem(UniqueFile(ctx, dir, "untitled", ext))},
		Buttons: []string{"Create", "Cancel"},
		Submit: func(v []string) {
			name := strings.TrimSpace(v[0])
			if !ValidName(name) {
				ctx.Notify(context.LevelError, name+" is not a valid file name")
				return
			}
			rel := path.Join(dir, name+ext)
			if Exists(ctx, rel) {
				ctx.Notify(context.LevelError, rel+" already exists")
				return
			}
			if err := create(rel); err != nil {
				ctx.Notify(context.LevelError, err.Error())
				return
			}
			p.created(ctx, rel)
		},
	})
}

func (p *Panel) createScript(ctx *context.Editor, dir, name string) {
	rel, err := CreateScript(ctx, dir, strings.TrimSpace(name))
	if err != nil {
		ctx.Notify(context.LevelError, err.Error())
		return
	}
	p.created(ctx, rel)
	if ctx.OpenScript != nil {
		ctx.OpenScript(rel, 3)
	}
}

func (p *Panel) newScript(ctx *context.Editor, dir string) {
	if rel, ok := scriptsRel(dir); ok && rel != "" {
		ctx.Ask(context.Question{
			Title:   "New script in package " + PackageOf(ctx, dir),
			Fields:  []string{"Script name"},
			Values:  []string{"NewScript"},
			Buttons: []string{"Create", "Cancel"},
			Submit:  func(v []string) { p.createScript(ctx, dir, v[0]) },
		})
		return
	}
	ctx.Ask(context.Question{
		Title:   "New script",
		Message: "Scripts live in a package folder under scripts/",
		Fields:  []string{"Package", "Script name"},
		Values:  []string{"", "NewScript"},
		Buttons: []string{"Create", "Cancel"},
		Submit: func(v []string) {
			pkg := strings.TrimSpace(v[0])
			if !ValidIdent(pkg) {
				ctx.Notify(context.LevelError, pkg+" is not a valid package name")
				return
			}
			p.createScript(ctx, path.Join("scripts", pkg), v[1])
		},
	})
}

func (p *Panel) newPackage(ctx *context.Editor, dir string) {
	base := "scripts"
	if _, ok := scriptsRel(dir); ok {
		base = dir
	}
	ctx.Ask(context.Question{
		Title:   "New script package",
		Message: "Creates " + base + "/<package>/ with its first script",
		Fields:  []string{"Package", "First script"},
		Values:  []string{"", "NewScript"},
		Buttons: []string{"Create", "Cancel"},
		Submit: func(v []string) {
			pkg := strings.TrimSpace(v[0])
			if !ValidIdent(pkg) {
				ctx.Notify(context.LevelError, pkg+" is not a valid package name")
				return
			}
			pkgDir := path.Join(base, pkg)
			if Exists(ctx, pkgDir) {
				ctx.Notify(context.LevelError, pkgDir+" already exists")
				return
			}
			if err := ctx.Pipeline.Mkdir(pkgDir); err != nil {
				ctx.Notify(context.LevelError, err.Error())
				return
			}
			p.expanded[base] = true
			p.createScript(ctx, pkgDir, v[1])
		},
	})
}

func (p *Panel) deleteDir(ctx *context.Editor, dir string) {
	var gone []ids.AssetID
	for i := range p.items {
		if Under(p.items[i].Path, dir) {
			gone = append(gone, p.items[i].ID)
		}
	}
	msg := "Delete the empty folder " + dir + "?"
	if n := len(gone); n > 0 {
		msg = "Delete " + dir + " and the " + strconv.Itoa(n) + " file(s) inside it? Scenes that reference them show them as missing."
	}
	ctx.Ask(context.Question{
		Title:   "Delete folder",
		Message: msg,
		Buttons: []string{"Delete", "Cancel"},
		Then: func(choice int) {
			if choice != 0 {
				return
			}
			err := DeleteDir(ctx, dir)
			for _, id := range gone {
				p.thumbs.Forget(id)
				delete(p.sel, id)
			}
			if p.folder == dir || Under(p.folder, dir) {
				p.folder = path.Dir(dir)
			}
			if err != nil {
				ctx.Notify(context.LevelError, err.Error())
			}
			p.invalidate()
		},
	})
}

func (p *Panel) DeleteSelected(ctx *context.Editor) {
	list := p.Selection()
	if len(list) == 0 {
		return
	}
	title, msg := "Delete asset", ""
	if len(list) == 1 {
		rel, _ := ctx.Pipeline.PathOf(list[0])
		msg = "Delete " + rel + "?"
	} else {
		title, msg = "Delete assets", "Delete "+strconv.Itoa(len(list))+" assets?"
	}
	ctx.Ask(context.Question{
		Title:   title,
		Message: msg + " Scenes that reference them keep the reference and show it as missing.",
		Buttons: []string{"Delete", "Cancel"},
		Then: func(choice int) {
			if choice != 0 {
				return
			}
			err := Delete(ctx, list)
			for _, id := range list {
				p.thumbs.Forget(id)
				delete(p.sel, id)
			}
			p.active = ids.AssetID{}
			if err != nil {
				ctx.Notify(context.LevelError, err.Error())
			}
			p.invalidate()
		},
	})
}

func (p *Panel) DuplicateSelected(ctx *context.Editor) error {
	var made []ids.AssetID
	var first error
	scripts := false
	for _, id := range p.Selection() {
		dup, err := Duplicate(ctx, id)
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		made = append(made, dup)
		if rel, ok := ctx.Pipeline.PathOf(dup); ok && path.Ext(rel) == document.ScriptExt {
			scripts = true
		}
	}
	if len(made) > 0 {
		clear(p.sel)
		for _, id := range made {
			p.sel[id] = true
		}
		p.active, p.anchor = made[len(made)-1], made[0]
	}
	if scripts {
		ctx.Notify(context.LevelWarn, "Rename the script types in the copy, a package cannot declare a script twice")
	}
	p.invalidate()
	return first
}

func (p *Panel) ReimportSelected(ctx *context.Editor) error {
	if ctx.Pipeline == nil {
		return nil
	}
	var first error
	for _, id := range p.Selection() {
		batch, err := ctx.Pipeline.Reimport(id)
		ctx.ApplyBatch(batch)
		if err != nil && first == nil {
			first = err
		}
	}
	if first == nil && len(p.sel) > 0 {
		ctx.Notify(context.LevelInfo, "Reimported")
	}
	return first
}

func SetStartup(ctx *context.Editor, id ids.AssetID, rel string) {
	if ctx.Project == nil {
		return
	}
	prev := ctx.Project.MainScene
	ctx.Project.MainScene = id
	if err := ctx.Project.Save(); err != nil {
		ctx.Project.MainScene = prev
		ctx.Notify(context.LevelError, err.Error())
		return
	}
	ctx.Notify(context.LevelInfo, "Startup scene: "+rel)
}
