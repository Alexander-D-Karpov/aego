package assets

import (
	favs "aego/editor/core/favorites"
	"aego/editor/core/refs"
	"aego/engine/i18n"
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
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.open", "Open"), "", "file.open", single && openable(a.Kind), false) {
		p.open(a)
	}
	widgets.Separator(ui)
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.copy", "Copy"), reg.Shortcut(CommandCopy), "edit.copy", true, false) {
		p.report(p.CopySelected())
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.cut", "Cut"), reg.Shortcut(CommandCut), "edit.cut", true, false) {
		p.report(p.CutSelected())
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.paste", "Paste"), reg.Shortcut(CommandPaste), "edit.paste", p.d.Files.CanPaste(), false) {
		p.report(p.Paste(path.Dir(a.Path)))
	}
	widgets.Separator(ui)
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.rename", "Rename"), reg.Shortcut(CommandRename), "edit.rename", single, false) {
		p.StartRename()
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.batch_rename", "Batch Rename…"), "", "edit.batch_rename", len(p.sel) > 1 && p.d.BatchRename != nil, false) {
		p.d.BatchRename(p.fileSelection())
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.duplicate", "Duplicate"), reg.Shortcut(CommandDuplicate), "edit.duplicate", true, false) {
		p.report(p.DuplicateSelected())
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.delete", "Delete"), reg.Shortcut(CommandDelete), "edit.delete", true, false) {
		p.DeleteSelected()
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.reimport", "Reimport"), "", "ui.reset", true, false) {
		p.report(p.ReimportSelected())
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.find_references", "Find References"), reg.Shortcut(CommandFindReferences), "panel.references", single && p.d.ShowReferences != nil, false) {
		p.d.ShowReferences(a.ID, nil)
	}
	if p.d.Favorites != nil && single {
		fav := favs.Item{Kind: favs.Asset, Asset: a.ID, Label: path.Base(a.Path)}
		label := i18n.Tr("editor.assets.add_to_favorites", "Add to Favorites")
		if p.d.Favorites.Has(fav) {
			label = i18n.Tr("editor.assets.remove_from_favorites", "Remove from Favorites")
		}
		if widgets.MenuItemIcon(ui, label, "", "ui.star", true, false) {
			_, err := p.d.Favorites.Toggle(fav)
			p.report(err)
		}
	}
	if single {
		id, rel, name := a.ID, a.Path, a.Name
		switch a.Kind {
		case asset.ArtScene:
			widgets.Separator(ui)
			startup := p.d.Project != nil && p.d.Project.MainScene == id
			if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.set_as_startup_scene", "Set as Startup Scene"), "", "play.start", !startup, startup) {
				p.d.Files.SetStartup(id, rel)
			}
		case asset.ArtPrefab:
			widgets.Separator(ui)
			if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.instantiate_in_scene", "Instantiate in Scene"), "", "file.prefab", p.d.Docs.Doc() != nil && p.Instantiate != nil, false) {
				p.Instantiate(id, path.Base(name))
			}
			if p.d.Prefabs != nil {
				if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.create_variant", "Create Variant"), "", "prefab.variant", true, false) {
					p.report(p.d.Prefabs.CreateVariant(id))
					p.invalidate()
				}
				if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.find_instances", "Find Instances"), "", "ui.search", true, false) {
					p.d.Prefabs.FindInstances(id)
				}
			}
		case asset.ArtScript:
			widgets.Separator(ui)
			nodes := p.d.Selection.Selected()
			if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.attach_to_selected", "Attach to Selected"), "", "comp.script", len(nodes) > 0, false) {
				p.d.Scenes.AttachScript(nodes, id)
			}
		}
	}
	widgets.Separator(ui)
	if p.d.Compare != nil && single && diffable(a.Path) {
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.compare_with_head", "Compare with HEAD"), "", "panel.diff", true, false) {
			p.d.Compare(a.Path)
		}
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.reveal_in_file_manager", "Reveal in File Manager"), "", "file.reveal", single, false) {
		p.report(reveal(p.d.Pipeline.Abs(a.Path), false))
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.copy_path", "Copy Path"), "", "edit.copy_path", true, false) {
		var paths []string
		for _, id := range p.fileSelection() {
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
	if widgets.SubMenuIcon(ui, i18n.Tr("editor.assets.new", "New"), "ui.plus", 200) {
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.folder", "Folder"), "", "file.folder", true, false) {
			p.newFolder(dir)
		}
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.scene", "Scene…"), "", "file.scene", p.d.Files.CanNewScene(), false) {
			p.newDoc(dir, i18n.Tr("editor.assets.new_scene", "New scene"), document.SceneExt, p.d.Files.NewScene)
		}
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.prefab", "Prefab…"), "", "file.prefab", p.d.Files.CanNewPrefab(), false) {
			p.newDoc(dir, i18n.Tr("editor.assets.new_prefab", "New prefab"), assetops.PrefabExt, p.d.Files.NewPrefab)
		}
		widgets.Separator(ui)
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.script", "Script…"), "", "file.script", true, false) {
			p.newScript(dir)
		}
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.script_package", "Script Package…"), "", "file.package", true, false) {
			p.newPackage(dir)
		}
		p.dataItems(dir)
		widgets.EndSubMenu(ui)
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.paste", "Paste"), reg.Shortcut(CommandPaste), "edit.paste", p.d.Files.CanPaste(), false) {
		p.report(p.Paste(dir))
	}
	widgets.Separator(ui)
	top := path.Dir(dir) == "."
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.rename", "Rename"), "", "edit.rename", !top, false) {
		p.expanded[path.Dir(dir)] = true
		p.renamingDir, p.renameDirStart = dir, true
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.delete_2", "Delete…"), "", "edit.delete", !top, false) {
		p.deleteDir(dir)
	}
	widgets.Separator(ui)
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.reveal_in_file_manager", "Reveal in File Manager"), "", "file.reveal", true, false) {
		p.report(reveal(p.d.Pipeline.Abs(dir), true))
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.assets.copy_path", "Copy Path"), "", "edit.copy_path", true, false) {
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
	ext, title := ".aedata", i18n.Tr("editor.assets.new_fullname_data", "New {fullname} data", i18n.Str("fullname", typeShort(td.FullName)))
	if table {
		ext, title = ".aetable", i18n.Tr("editor.assets.new_fullname_table", "New {fullname} table", i18n.Str("fullname", typeShort(td.FullName)))
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
	if widgets.SubMenuIcon(ui, i18n.Tr("editor.assets.data_asset", "Data Asset"), "file.data", 220) {
		for _, td := range types {
			if widgets.MenuItemIcon(ui, typeShort(td.FullName)+"…", "", "file.data", true, false) {
				p.newData(dir, td, false)
			}
		}
		widgets.EndSubMenu(ui)
	}
	if widgets.SubMenuIcon(ui, i18n.Tr("editor.assets.data_table", "Data Table"), "file.table", 220) {
		for _, td := range types {
			if widgets.MenuItemIcon(ui, typeShort(td.FullName)+"…", "", "file.table", true, false) {
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
		Fields:  []string{i18n.Tr("editor.assets.name", "Name")},
		Values:  []string{assetops.Stem(p.d.Files.UniqueFile(dir, "untitled", ext))},
		Buttons: []string{i18n.Tr("editor.assets.create", "Create"), i18n.Tr("editor.assets.cancel", "Cancel")},
		Submit: func(v []string) {
			name := strings.TrimSpace(v[0])
			if !assetops.ValidName(name) {
				p.d.Notices.Notify(context.LevelError, i18n.Note("editor.assets.name_is_not_a_valid", "{name} is not a valid file name", i18n.Str("name", name)))
				return
			}
			rel := path.Join(dir, name+ext)
			if p.d.Files.Exists(rel) {
				p.d.Notices.Notify(context.LevelError, i18n.Note("editor.assets.rel_already_exists", "{rel} already exists", i18n.Str("rel", rel)))
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
			Title:   i18n.Tr("editor.assets.new_script_in_package_dir", "New script in package {dir}", i18n.Str("dir", p.d.Files.PackageOf(dir))),
			Fields:  []string{i18n.Tr("editor.assets.script_name", "Script name")},
			Values:  []string{"NewScript"},
			Buttons: []string{i18n.Tr("editor.assets.create", "Create"), i18n.Tr("editor.assets.cancel", "Cancel")},
			Submit:  func(v []string) { p.createScript(dir, v[0]) },
		})
		return
	}
	p.d.Notices.Ask(context.Question{
		Title:   i18n.Tr("editor.assets.new_script", "New script"),
		Message: i18n.Tr("editor.assets.scripts_live_in_a_package", "Scripts live in a package folder under scripts/"),
		Fields:  []string{i18n.Tr("editor.assets.package", "Package"), i18n.Tr("editor.assets.script_name", "Script name")},
		Values:  []string{"", "NewScript"},
		Buttons: []string{i18n.Tr("editor.assets.create", "Create"), i18n.Tr("editor.assets.cancel", "Cancel")},
		Submit: func(v []string) {
			pkg := strings.TrimSpace(v[0])
			if !assetops.ValidIdent(pkg) {
				p.d.Notices.Notify(context.LevelError, i18n.Note("editor.assets.pkg_is_not_a_valid", "{pkg} is not a valid package name", i18n.Str("pkg", pkg)))
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
		Title:   i18n.Tr("editor.assets.new_script_package", "New script package"),
		Message: "Creates " + base + "/<package>/ with its first script",
		Fields:  []string{i18n.Tr("editor.assets.package", "Package"), i18n.Tr("editor.assets.first_script", "First script")},
		Values:  []string{"", "NewScript"},
		Buttons: []string{i18n.Tr("editor.assets.create", "Create"), i18n.Tr("editor.assets.cancel", "Cancel")},
		Submit: func(v []string) {
			pkg := strings.TrimSpace(v[0])
			if !assetops.ValidIdent(pkg) {
				p.d.Notices.Notify(context.LevelError, i18n.Note("editor.assets.pkg_is_not_a_valid", "{pkg} is not a valid package name", i18n.Str("pkg", pkg)))
				return
			}
			pkgDir := path.Join(base, pkg)
			if p.d.Files.Exists(pkgDir) {
				p.d.Notices.Notify(context.LevelError, i18n.Note("editor.assets.pkgdir_already_exists", "{pkgdir} already exists", i18n.Str("pkgdir", pkgDir)))
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
	msg := i18n.Tr("editor.assets.delete_the_empty_folder_dir", "Delete the empty folder {dir}?", i18n.Str("dir", dir))
	if n := len(gone); n > 0 {
		msg = i18n.Tr("editor.assets.delete_dir_and_the_n", "Delete {dir} and the {n} file(s) inside it? Scenes that reference them show them as missing.", i18n.Str("dir", dir), i18n.Str("n", strconv.Itoa(n)))
	}
	p.d.Notices.Ask(context.Question{
		Title:   i18n.Tr("editor.assets.delete_folder", "Delete folder"),
		Message: msg,
		Buttons: []string{i18n.Tr("editor.assets.delete", "Delete"), i18n.Tr("editor.assets.cancel", "Cancel")},
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
	list := p.fileSelection()
	if len(list) == 0 {
		return
	}
	if p.d.Refs != nil && p.d.ShowReferences != nil {
		gone := map[ids.AssetID]bool{}
		for _, id := range list {
			gone[id] = true
		}
		var users []refs.Ref
		var first ids.AssetID
		for _, id := range list {
			for _, r := range p.d.Refs.Referrers(id) {
				if r.Project || !gone[r.Asset] {
					users = append(users, r)
					if first.IsZero() {
						first = id
					}
				}
			}
		}
		if len(users) > 0 {
			p.askUsed(list, users, first)
			return
		}
	}
	title, msg := i18n.Tr("editor.assets.delete_asset", "Delete asset"), ""
	if len(list) == 1 {
		rel, _ := p.d.Pipeline.PathOf(list[0])
		msg = i18n.Tr("editor.assets.delete_rel", "Delete {rel}?", i18n.Str("rel", rel))
	} else {
		title, msg = i18n.Tr("editor.assets.delete_assets", "Delete assets"), i18n.Tr("editor.assets.delete_list_assets", "Delete {list} assets?", i18n.Str("list", strconv.Itoa(len(list))))
	}
	p.d.Notices.Ask(context.Question{
		Title:   title,
		Message: i18n.Tr("editor.assets.msg_nothing_in_the_project", "{msg} Nothing in the project references them.", i18n.Str("msg", msg)),
		Buttons: []string{i18n.Tr("editor.assets.delete", "Delete"), i18n.Tr("editor.assets.cancel", "Cancel")},
		Then: func(choice int) {
			if choice == 0 {
				p.DeleteNow(list)
			}
		},
	})
}

func (p *Panel) DuplicateSelected() error {
	var made []ids.AssetID
	var first error
	scripts := false
	for _, id := range p.fileSelection() {
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
		p.d.Notices.Notify(context.LevelWarn, i18n.Note("editor.assets.rename_the_script_types_in", "Rename the script types in the copy, a package cannot declare a script twice"))
	}
	p.invalidate()
	return first
}

func (p *Panel) ReimportSelected() error {
	if p.d.Pipeline == nil {
		return nil
	}
	var first error
	for _, id := range p.fileSelection() {
		err := p.d.Files.Reimport(id)
		if err != nil && first == nil {
			first = err
		}
	}
	if first == nil && len(p.sel) > 0 {
		p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.assets.reimported", "Reimported"))
	}
	return first
}

func (p *Panel) askUsed(list []ids.AssetID, users []refs.Ref, first ids.AssetID) {
	var names []string
	for i, r := range users {
		if i == 8 {
			names = append(names, "and "+strconv.Itoa(len(users)-i)+" more")
			break
		}
		names = append(names, r.Path)
	}
	p.d.Notices.Ask(context.Question{
		Title:   i18n.Tr("editor.assets.assets_in_use", "Assets in use"),
		Message: i18n.Tr("editor.assets.still_referenced_by_join_deleting", "Still referenced by:\n{join}\n\nDeleting them leaves these references missing.", i18n.Str("join", strings.Join(names, "\n"))),
		Buttons: []string{i18n.Tr("editor.assets.show_references", "Show References"), i18n.Tr("editor.assets.delete_anyway", "Delete Anyway"), i18n.Tr("editor.assets.cancel", "Cancel")},
		Then: func(choice int) {
			switch choice {
			case 0:
				p.d.ShowReferences(first, list)
			case 1:
				p.DeleteNow(list)
			}
		},
	})
}

func (p *Panel) DeleteNow(list []ids.AssetID) {
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
}

func diffable(rel string) bool {
	switch path.Ext(rel) {
	case ".aescene", ".aeprefab", ".aedata", ".aetable", ".aepreset":
		return true
	}
	return false
}
