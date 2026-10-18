package assets

import (
	"path"
	"strconv"
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/data"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/schema"
	"aego/ui/widgets"
)

func (p *Panel) menus() {
	ui := p.f.UI
	if widgets.ContextMenu(ui, p.assetMenuID) {
		p.assetItems()
		widgets.EndContextMenu(ui)
	}
	if widgets.ContextMenu(ui, p.dirMenuID) {
		p.dirItems(p.menuDir)
		widgets.EndContextMenu(ui)
	}
}

func (p *Panel) copyText(s string) {
	if c := p.d.Files.Clipboard(); c != nil {
		c.SetText(s)
	}
}

func (p *Panel) assetItems() {
	ui := p.f.UI
	a := p.find(p.active)
	if a == nil || !p.sel[a.ID] {
		return
	}
	single := len(p.sel) == 1
	reg := p.d.Commands
	if widgets.MenuItem(ui, "Open", "", single && openable(a.Kind), false) {
		p.open(a)
	}
	widgets.Separator(ui)
	if widgets.MenuItem(ui, "Copy", reg.Shortcut(CommandCopy), true, false) {
		p.report(p.CopySelected())
	}
	if widgets.MenuItem(ui, "Cut", reg.Shortcut(CommandCut), true, false) {
		p.report(p.CutSelected())
	}
	if widgets.MenuItem(ui, "Paste", reg.Shortcut(CommandPaste), p.d.Files.CanPaste(), false) {
		p.report(p.Paste(path.Dir(a.Path)))
	}
	widgets.Separator(ui)
	if widgets.MenuItem(ui, "Rename", reg.Shortcut(CommandRename), single, false) {
		p.StartRename()
	}
	if widgets.MenuItem(ui, "Duplicate", reg.Shortcut(CommandDuplicate), true, false) {
		p.report(p.DuplicateSelected())
	}
	if widgets.MenuItem(ui, "Delete", reg.Shortcut(CommandDelete), true, false) {
		p.DeleteSelected()
	}
	if widgets.MenuItem(ui, "Reimport", "", true, false) {
		p.report(p.ReimportSelected())
	}
	if single {
		id, rel, name := a.ID, a.Path, a.Name
		switch a.Kind {
		case asset.ArtScene:
			widgets.Separator(ui)
			startup := p.d.Project != nil && p.d.Project.MainScene == id
			if widgets.MenuItem(ui, "Set as Startup Scene", "", !startup, startup) {
				p.d.Files.SetStartup(id, rel)
			}
		case asset.ArtPrefab:
			widgets.Separator(ui)
			if widgets.MenuItem(ui, "Instantiate in Scene", "", p.d.Docs.Doc() != nil && p.Instantiate != nil, false) {
				p.Instantiate(id, path.Base(name))
			}
			if p.d.Prefabs != nil {
				if widgets.MenuItem(ui, "Create Variant", "", true, false) {
					p.report(p.d.Prefabs.CreateVariant(id))
					p.invalidate()
				}
				if widgets.MenuItem(ui, "Find Instances", "", true, false) {
					p.d.Prefabs.FindInstances(id)
				}
			}
		case asset.ArtScript:
			widgets.Separator(ui)
			nodes := p.d.Selection.Selected()
			if widgets.MenuItem(ui, "Attach to Selected", "", len(nodes) > 0, false) {
				p.d.Scenes.AttachScript(nodes, id)
			}
		}
	}
	widgets.Separator(ui)
	if widgets.MenuItem(ui, "Reveal in File Manager", "", single, false) {
		p.report(reveal(p.d.Pipeline.Abs(a.Path), false))
	}
	if widgets.MenuItem(ui, "Copy Path", "", true, false) {
		var paths []string
		for _, id := range p.Selection() {
			if rel, ok := p.d.Pipeline.PathOf(id); ok {
				paths = append(paths, rel)
			}
		}
		p.copyText(strings.Join(paths, "\n"))
	}
}

func (p *Panel) dirItems(dir string) {
	ui := p.f.UI
	reg := p.d.Commands
	if widgets.SubMenu(ui, "New", 200) {
		if widgets.MenuItem(ui, "Folder", "", true, false) {
			p.newFolder(dir)
		}
		if widgets.MenuItem(ui, "Scene…", "", p.d.Files.CanNewScene(), false) {
			p.newDoc(dir, "New scene", document.SceneExt, p.d.Files.NewScene)
		}
		if widgets.MenuItem(ui, "Prefab…", "", p.d.Files.CanNewPrefab(), false) {
			p.newDoc(dir, "New prefab", assetops.PrefabExt, p.d.Files.NewPrefab)
		}
		widgets.Separator(ui)
		if widgets.MenuItem(ui, "Script…", "", true, false) {
			p.newScript(dir)
		}
		if widgets.MenuItem(ui, "Script Package…", "", true, false) {
			p.newPackage(dir)
		}
		p.dataItems(dir)
		widgets.EndSubMenu(ui)
	}
	if widgets.MenuItem(ui, "Paste", reg.Shortcut(CommandPaste), p.d.Files.CanPaste(), false) {
		p.report(p.Paste(dir))
	}
	widgets.Separator(ui)
	top := path.Dir(dir) == "."
	if widgets.MenuItem(ui, "Rename", "", !top, false) {
		p.expanded[path.Dir(dir)] = true
		p.renamingDir, p.renameDirStart = dir, true
	}
	if widgets.MenuItem(ui, "Delete…", "", !top, false) {
		p.deleteDir(dir)
	}
	widgets.Separator(ui)
	if widgets.MenuItem(ui, "Reveal in File Manager", "", true, false) {
		p.report(reveal(p.d.Pipeline.Abs(dir), true))
	}
	if widgets.MenuItem(ui, "Copy Path", "", true, false) {
		p.copyText(dir)
	}
}

func (p *Panel) dataTypes() []*schema.TypeDescriptor {
	if p.d.Schema == nil {
		return nil
	}
	var out []*schema.TypeDescriptor
	p.d.Schema.Each(func(td *schema.TypeDescriptor) {
		if td.Kind == schema.TypeAsset && !td.Hidden {
			out = append(out, td)
		}
	})
	return out
}

func typeShort(full string) string {
	return full[strings.LastIndexByte(full, '.')+1:]
}

func (p *Panel) newData(dir string, td *schema.TypeDescriptor, table bool) {
	ext, title := ".aedata", "New "+typeShort(td.FullName)+" data"
	if table {
		ext, title = ".aetable", "New "+typeShort(td.FullName)+" table"
	}
	p.newDoc(dir, title, ext, func(rel string) error {
		var b []byte
		if table {
			b = data.WriteTable(&data.TableDoc{Type: td.FullName})
		} else {
			b = data.WriteDoc(&data.Doc{Type: td.FullName})
		}
		_, err := p.d.Files.CreateFile(rel, b)
		return err
	})
}

func (p *Panel) dataItems(dir string) {
	types := p.dataTypes()
	if len(types) == 0 {
		return
	}
	ui := p.f.UI
	widgets.Separator(ui)
	if widgets.SubMenu(ui, "Data Asset", 220) {
		for _, td := range types {
			if widgets.MenuItem(ui, typeShort(td.FullName)+"…", "", true, false) {
				p.newData(dir, td, false)
			}
		}
		widgets.EndSubMenu(ui)
	}
	if widgets.SubMenu(ui, "Data Table", 220) {
		for _, td := range types {
			if widgets.MenuItem(ui, typeShort(td.FullName)+"…", "", true, false) {
				p.newData(dir, td, true)
			}
		}
		widgets.EndSubMenu(ui)
	}
}

func (p *Panel) created(rel string) {
	p.invalidate()
	p.refresh()
	p.folder = path.Dir(rel)
	p.search = ""
	p.expanded[path.Dir(p.folder)] = true
	if id, ok := p.d.Pipeline.Lookup(rel); ok {
		p.selectOnly(id)
		p.scrollTo = id
	}
}

func (p *Panel) newDoc(dir, title, ext string, create func(string) error) {
	p.d.Notices.Ask(context.Question{
		Title:   title,
		Fields:  []string{"Name"},
		Values:  []string{assetops.Stem(p.d.Files.UniqueFile(dir, "untitled", ext))},
		Buttons: []string{"Create", "Cancel"},
		Submit: func(v []string) {
			name := strings.TrimSpace(v[0])
			if !assetops.ValidName(name) {
				p.d.Notices.Notify(context.LevelError, name+" is not a valid file name")
				return
			}
			rel := path.Join(dir, name+ext)
			if p.d.Files.Exists(rel) {
				p.d.Notices.Notify(context.LevelError, rel+" already exists")
				return
			}
			if err := create(rel); err != nil {
				p.d.Notices.Notify(context.LevelError, err.Error())
				return
			}
			p.created(rel)
		},
	})
}

func (p *Panel) createScript(dir, name string) {
	rel, err := p.d.Files.CreateScript(dir, strings.TrimSpace(name))
	if err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
		return
	}
	p.created(rel)
	if p.d.Files.ScriptEditor() {
		p.d.Nav.Open(context.At(rel, 3, 1))
	}
}

func (p *Panel) newScript(dir string) {
	if rel, ok := assetops.ScriptsRel(dir); ok && rel != "" {
		p.d.Notices.Ask(context.Question{
			Title:   "New script in package " + p.d.Files.PackageOf(dir),
			Fields:  []string{"Script name"},
			Values:  []string{"NewScript"},
			Buttons: []string{"Create", "Cancel"},
			Submit:  func(v []string) { p.createScript(dir, v[0]) },
		})
		return
	}
	p.d.Notices.Ask(context.Question{
		Title:   "New script",
		Message: "Scripts live in a package folder under scripts/",
		Fields:  []string{"Package", "Script name"},
		Values:  []string{"", "NewScript"},
		Buttons: []string{"Create", "Cancel"},
		Submit: func(v []string) {
			pkg := strings.TrimSpace(v[0])
			if !assetops.ValidIdent(pkg) {
				p.d.Notices.Notify(context.LevelError, pkg+" is not a valid package name")
				return
			}
			p.createScript(path.Join("scripts", pkg), v[1])
		},
	})
}

func (p *Panel) newPackage(dir string) {
	base := "scripts"
	if _, ok := assetops.ScriptsRel(dir); ok {
		base = dir
	}
	p.d.Notices.Ask(context.Question{
		Title:   "New script package",
		Message: "Creates " + base + "/<package>/ with its first script",
		Fields:  []string{"Package", "First script"},
		Values:  []string{"", "NewScript"},
		Buttons: []string{"Create", "Cancel"},
		Submit: func(v []string) {
			pkg := strings.TrimSpace(v[0])
			if !assetops.ValidIdent(pkg) {
				p.d.Notices.Notify(context.LevelError, pkg+" is not a valid package name")
				return
			}
			pkgDir := path.Join(base, pkg)
			if p.d.Files.Exists(pkgDir) {
				p.d.Notices.Notify(context.LevelError, pkgDir+" already exists")
				return
			}
			if err := p.d.Pipeline.Mkdir(pkgDir); err != nil {
				p.d.Notices.Notify(context.LevelError, err.Error())
				return
			}
			p.expanded[base] = true
			p.createScript(pkgDir, v[1])
		},
	})
}

func (p *Panel) deleteDir(dir string) {
	var gone []ids.AssetID
	for i := range p.items {
		if assetops.Under(p.items[i].Path, dir) {
			gone = append(gone, p.items[i].ID)
		}
	}
	msg := "Delete the empty folder " + dir + "?"
	if n := len(gone); n > 0 {
		msg = "Delete " + dir + " and the " + strconv.Itoa(n) + " file(s) inside it? Scenes that reference them show them as missing."
	}
	p.d.Notices.Ask(context.Question{
		Title:   "Delete folder",
		Message: msg,
		Buttons: []string{"Delete", "Cancel"},
		Then: func(choice int) {
			if choice != 0 {
				return
			}
			err := p.d.Files.DeleteDir(dir)
			for _, id := range gone {
				p.d.Thumbs.Forget(id)
				delete(p.sel, id)
			}
			if p.folder == dir || assetops.Under(p.folder, dir) {
				p.folder = path.Dir(dir)
			}
			if err != nil {
				p.d.Notices.Notify(context.LevelError, err.Error())
			}
			p.invalidate()
		},
	})
}

func (p *Panel) DeleteSelected() {
	list := p.Selection()
	if len(list) == 0 {
		return
	}
	title, msg := "Delete asset", ""
	if len(list) == 1 {
		rel, _ := p.d.Pipeline.PathOf(list[0])
		msg = "Delete " + rel + "?"
	} else {
		title, msg = "Delete assets", "Delete "+strconv.Itoa(len(list))+" assets?"
	}
	p.d.Notices.Ask(context.Question{
		Title:   title,
		Message: msg + " Scenes that reference them keep the reference and show it as missing.",
		Buttons: []string{"Delete", "Cancel"},
		Then: func(choice int) {
			if choice != 0 {
				return
			}
			err := p.d.Files.Delete(list)
			for _, id := range list {
				p.d.Thumbs.Forget(id)
				delete(p.sel, id)
			}
			p.active = ids.AssetID{}
			if err != nil {
				p.d.Notices.Notify(context.LevelError, err.Error())
			}
			p.invalidate()
		},
	})
}

func (p *Panel) DuplicateSelected() error {
	var made []ids.AssetID
	var first error
	scripts := false
	for _, id := range p.Selection() {
		dup, err := p.d.Files.Duplicate(id)
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		made = append(made, dup)
		if rel, ok := p.d.Pipeline.PathOf(dup); ok && path.Ext(rel) == document.ScriptExt {
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
		p.d.Notices.Notify(context.LevelWarn, "Rename the script types in the copy, a package cannot declare a script twice")
	}
	p.invalidate()
	return first
}

func (p *Panel) ReimportSelected() error {
	if p.d.Pipeline == nil {
		return nil
	}
	var first error
	for _, id := range p.Selection() {
		err := p.d.Files.Reimport(id)
		if err != nil && first == nil {
			first = err
		}
	}
	if first == nil && len(p.sel) > 0 {
		p.d.Notices.Notify(context.LevelInfo, "Reimported")
	}
	return first
}
