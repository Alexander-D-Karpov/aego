package editor

import (
	"bytes"
	"fmt"
	"path"
	"slices"
	"strings"

	"aego/art"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/artedit"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/scene/mutate"
	"aego/schema"
)

// A per-object copy snapshots the current document, including unsaved pixels.
// Animation clips and controllers are copied only when their typed references
// need remapping. Their timing, events and gameplay fields remain intact.
func askDuplicateSpriteSource(ed *context.Editor, node ids.NodeID, selected ids.AssetID) error {
	if ed == nil || ed.Doc() == nil || ed.Doc().IsPlay() || !canEditSourceArt(ed, selected) {
		return fmt.Errorf("%s", i18n.Tr("editor.art.duplicate_edit_scene", "Duplicate for This Object requires an editable scene and a project image."))
	}
	sceneDoc, docs, pl := ed.Doc(), ed.Docs, ed.Pipeline
	sc, revision := sceneDoc.Scene(), sceneDoc.History.Version()
	p := &artDuplication{ed: ed, guards: map[string][]byte{}, ids: map[ids.AssetID]ids.AssetID{}, visiting: map[ids.AssetID]bool{}, planned: map[string]bool{}}
	source, err := ed.DocumentForAsset(selected)
	if err != nil {
		return err
	}
	encoded, sourceMeta, frames, err := p.art(source)
	if err != nil {
		return err
	}
	// Read the object's exact current bindings before presenting the proposal.
	var bindings []artObjectBinding
	for _, field := range [][2]string{{"engine.SpriteRenderer", "sprite"}, {"animation.AnimationPlayer", "clip"}, {"animation.Animator", "controller"}} {
		td, ok := sc.Registry().ByName(field[0])
		if !ok {
			continue
		}
		fd, ok := td.Field(field[1])
		if !ok {
			continue
		}
		value, err := sc.GetProperty(sc.Handle(node), td.ID, fd.ID)
		if err != nil || value.Kind != schema.KindAssetRef || value.Asset.IsZero() {
			continue
		}
		bindings = append(bindings, artObjectBinding{field: td.FullName + "." + fd.Name, id: value.Asset})
	}
	if len(bindings) == 0 || bindings[0].id != selected {
		return fmt.Errorf("%s", i18n.Tr("editor.art.duplicate_selection_changed", "The selected object's sprite changed. Open the action again."))
	}
	stem := strings.TrimSuffix(path.Base(source.Path), path.Ext(source.Path))
	ed.Ask(context.Question{
		Title:   i18n.Tr("editor.art.duplicate_for_object", "Duplicate for This Object"),
		Message: i18n.Tr("editor.art.duplicate_notice", "Create an independent art source from the current pixels, and copy any affected clips and controller for this object. Other objects keep the shared asset. Undo restores this object's references; the new files remain available in Assets."),
		Fields:  []string{i18n.Tr("editor.art.source_path", "Source path")},
		Values:  []string{assetops.UniqueFile(ed, path.Dir(source.Path), stem+" copy", art.SourceExtension)},
		Buttons: []string{i18n.Tr("editor.art.duplicate_for_object", "Duplicate for This Object"), i18n.Tr("editor.art.cancel", "Cancel")},
		Submit: func(values []string) {
			fail := func(err error) { ed.Notify(context.LevelError, err.Error()) }
			if len(values) != 1 || ed.Docs != docs || ed.Pipeline != pl || ed.Doc() != sceneDoc || sceneDoc.IsPlay() || sceneDoc.History.Version() != revision || !sc.IsAlive(sc.Handle(node)) {
				fail(fmt.Errorf("%s", i18n.Tr("editor.art.duplicate_selection_changed", "The selected object's sprite changed. Open the action again.")))
				return
			}
			for _, current := range p.current {
				if !current() {
					fail(art.ErrConflict)
					return
				}
			}
			target := strings.TrimSpace(values[0])
			if !strings.EqualFold(path.Ext(target), art.SourceExtension) {
				fail(fmt.Errorf("%s", i18n.Tr("editor.art.link_extension", "The editable source must use .aeart.")))
				return
			}
			meta, err := pl.NewSourceMeta(target)
			if err != nil {
				fail(err)
				return
			}
			meta.Settings, meta.Pivot, meta.HasPivot = sourceMeta.Settings.Clone(), sourceMeta.Pivot, sourceMeta.HasPivot
			p.base = strings.TrimSuffix(target, path.Ext(target))
			p.planned[target], p.planned[target+pipeline.MetaSuffix] = true, true
			p.edits = []pipeline.SourceEdit{{Path: target, After: encoded}, {Path: target + pipeline.MetaSuffix, After: pipeline.WriteMeta(meta)}}
			for old, frame := range frames {
				p.ids[old] = artsource.StableID(meta.ID, artsource.FrameName(frame, art.Albedo))
			}
			var ops []mutate.Op
			for _, binding := range bindings {
				id, err := p.animation(binding.id)
				if err != nil {
					fail(err)
					return
				}
				if id != binding.id {
					ops = append(ops, mutate.SetValue(commands.Ref(node), binding.field, aetext.AssetRef(id.String())))
				}
			}
			for _, current := range p.current {
				if !current() {
					fail(art.ErrConflict)
					return
				}
			}
			keys := make([]string, 0, len(p.guards))
			for name := range p.guards {
				keys = append(keys, name)
			}
			slices.Sort(keys)
			for _, name := range keys {
				p.edits = append(p.edits, pipeline.SourceEdit{Path: name, Before: p.guards[name], After: p.guards[name], ReadOnlyGuard: true})
			}
			batch, err := pl.ApplySources(p.edits, false)
			if err != nil {
				fail(err)
				return
			}
			ed.ApplyBatch(batch)
			// Import callbacks may change scene selection/history. The new files
			// remain available, but never assign into a changed scene.
			if ed.Docs != docs || ed.Pipeline != pl || ed.Doc() != sceneDoc || sceneDoc.IsPlay() || sceneDoc.History.Version() != revision || !sc.IsAlive(sc.Handle(node)) {
				fail(fmt.Errorf("art: scene changed; independent files remain available, but were not assigned"))
				return
			}
			// One existing Mutation Service command owns all object binding
			// changes. Art file creation never mutates colliders or materials.
			label := i18n.Tr("editor.art.duplicate_for_object", "Duplicate for This Object")
			if err := sceneDoc.History.Exec(commands.NewMutation(label, commands.EnvFor(sc), ops...)); err != nil {
				fail(err)
				return
			}
			if _, err := ed.OpenAsset(p.ids[selected]); err != nil {
				fail(err)
			}
		},
	})
	return nil
}

type artObjectBinding struct {
	field string
	id    ids.AssetID
}

type artDuplication struct {
	ed       *context.Editor
	guards   map[string][]byte
	current  []func() bool
	ids      map[ids.AssetID]ids.AssetID
	visiting map[ids.AssetID]bool
	planned  map[string]bool
	base     string
	edits    []pipeline.SourceEdit
}

func (p *artDuplication) read(name string) ([]byte, error) {
	if _, ok := p.guards[name]; !ok {
		data, err := p.ed.Pipeline.SourceBytes(name)
		if err != nil {
			return nil, fmt.Errorf("art: cannot snapshot source %s: %w", name, err)
		}
		if data == nil {
			return nil, fmt.Errorf("art: cannot snapshot missing source %s", name)
		}
		p.guards[name] = bytes.Clone(data)
	}
	if doc := p.ed.Docs.Script(name); doc != nil {
		if doc.Conflict {
			return nil, art.ErrConflict
		}
		revision := doc.Code.Rev()
		p.current = append(p.current, func() bool { return p.ed.Docs.Script(name) == doc && doc.Code.Rev() == revision && !doc.Conflict })
		return bytes.Clone(doc.Code.Bytes()), nil
	}
	return p.guards[name], nil
}

func (p *artDuplication) art(source *document.AssetDocument) ([]byte, *pipeline.Meta, map[ids.AssetID]art.ID, error) {
	encodedMeta, err := p.read(source.Path + pipeline.MetaSuffix)
	if err != nil {
		return nil, nil, nil, err
	}
	meta, err := pipeline.ParseMeta(encodedMeta, source.Path+pipeline.MetaSuffix)
	if err != nil {
		return nil, nil, nil, err
	}
	frames := map[ids.AssetID]art.ID{}
	if strings.EqualFold(path.Ext(source.Path), ".png") {
		link, err := artsource.TextureLinkOf(meta)
		if err != nil {
			return nil, nil, nil, err
		}
		if link != nil {
			linked, err := p.ed.DocumentForAsset(link.Source)
			if err != nil {
				return nil, nil, nil, err
			}
			encoded, artMeta, artFrames, err := p.art(linked)
			if err != nil {
				return nil, nil, nil, err
			}
			for _, f := range link.Frames {
				artFrames[f.Asset] = f.Frame
			}
			if len(link.Frames) != 0 {
				artFrames[source.Asset] = link.Frames[0].Frame
			}
			return encoded, artMeta, artFrames, err
		}
		png, err := p.read(source.Path)
		if err != nil {
			return nil, nil, nil, err
		}
		d, mapping, err := artsource.ImportTextureDocument(&pipeline.ImportContext{ID: source.Asset, Path: source.Path, Source: png, Meta: meta, Read: p.read, Lookup: p.ed.Pipeline.Lookup, SourcePath: p.ed.Pipeline.PathOf})
		if err != nil {
			return nil, nil, nil, err
		}
		defer d.Close()
		encoded, err := d.Encode()
		if model, ok := source.Binding.Model.(*artedit.Document); ok && model.Draft {
			if model.PendingRecovery || model.Session.Info().Gesture {
				return nil, nil, nil, fmt.Errorf("art: resolve recovery and finish the gesture before duplicating")
			}
			if err := model.CheckDraftInputs(); err != nil {
				return nil, nil, nil, err
			}
			available := map[art.ID]bool{}
			for _, frame := range model.Session.Document.Info().Frames {
				available[frame.ID] = true
			}
			for _, frame := range mapping {
				if !available[frame.Frame] {
					return nil, nil, nil, fmt.Errorf("art: an original image frame was removed; resolve its references before duplicating")
				}
			}
			var revision uint64
			encoded, revision, err = model.PreviewSnapshot()
			if err != nil {
				return nil, nil, nil, err
			}
			p.current = append(p.current, func() bool {
				return p.ed.Docs.AssetDocument(source.Asset) == source && !model.PendingRecovery && model.CheckDraftInputs() == nil && model.PreviewCurrent(revision)
			})
		}
		for _, f := range mapping {
			frames[f.Asset] = f.Frame
		}
		frames[source.Asset] = mapping[0].Frame
		// Embedded channels replace external surface references in the copy.
		settings := artsource.New().Defaults()
		for _, key := range []string{"filter", "colorspace", "srgb"} {
			if v, ok := meta.Settings[key]; ok {
				settings[key] = v
			}
		}
		meta.Settings = settings
		return encoded, meta, frames, err
	}
	encoded, err := p.read(source.Path)
	if err != nil {
		return nil, nil, nil, err
	}
	if model, ok := source.Binding.Model.(*artedit.Document); ok {
		if model.PendingRecovery {
			return nil, nil, nil, fmt.Errorf("art: recover or discard the pending recovery before duplicating")
		}
		if model.Session.Info().Gesture {
			return nil, nil, nil, fmt.Errorf("art: finish or cancel the current gesture before duplicating")
		}
		var revision uint64
		encoded, revision, err = model.PreviewSnapshot()
		if err != nil {
			return nil, nil, nil, err
		}
		p.current = append(p.current, func() bool {
			return p.ed.Docs.AssetDocument(source.Asset) == source && !model.PendingRecovery && model.PreviewCurrent(revision)
		})
	}
	d, err := art.Decode(encoded, art.DefaultLimits())
	if err != nil {
		return nil, nil, nil, err
	}
	defer d.Close()
	for _, f := range d.Info().Frames {
		name := artsource.FrameName(f.ID, art.Albedo)
		id := artsource.StableID(source.Asset, name)
		for _, sub := range meta.Subs {
			if sub.Name == name {
				id = sub.ID
			}
		}
		frames[id] = f.ID
	}
	frames[source.Asset] = d.Info().Frames[0].ID
	return encoded, meta, frames, nil
}

// animation recursively rewrites only typed asset references in the existing
// animation text formats. Source comments/formatting may change in the new
// copy; original files and their current unsaved buffers are never rewritten.
func (p *artDuplication) animation(id ids.AssetID) (ids.AssetID, error) {
	if mapped, ok := p.ids[id]; ok {
		return mapped, nil
	}
	name, ok := p.ed.Pipeline.PathOf(id)
	if !ok || (path.Ext(name) != ".aeanim" && path.Ext(name) != ".aectrl") {
		return id, nil
	}
	if p.visiting[id] || len(p.visiting) >= 256 {
		return id, fmt.Errorf("art: cyclic or excessive animation dependencies")
	}
	p.visiting[id] = true
	defer delete(p.visiting, id)
	data, err := p.read(name)
	if err != nil {
		return id, err
	}
	node, err := aetext.Parse(data, name)
	if err != nil {
		return id, err
	}
	changed := false
	var rewrite func(*aetext.Value) error
	rewrite = func(v *aetext.Value) error {
		switch v.Kind {
		case aetext.VAsset:
			old, err := ids.ParseAssetID(v.Str)
			if err != nil {
				return err
			}
			mapped, err := p.animation(old)
			if err != nil {
				return err
			}
			if mapped != old {
				*v, changed = aetext.AssetRef(mapped.String()), true
			}
		case aetext.VList:
			for i := range v.List {
				if err := rewrite(&v.List[i]); err != nil {
					return err
				}
			}
		case aetext.VStruct:
			for i := range v.Obj {
				if err := rewrite(&v.Obj[i].Value); err != nil {
					return err
				}
			}
		}
		return nil
	}
	var walk func(*aetext.Node) error
	walk = func(n *aetext.Node) error {
		for _, fields := range [][]aetext.Field{n.Attrs, n.Fields} {
			for i := range fields {
				if err := rewrite(&fields[i].Value); err != nil {
					return err
				}
			}
		}
		for _, child := range n.Blocks {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(node); err != nil || !changed {
		p.ids[id] = id
		return id, err
	}
	if len(p.edits) > 512 {
		return id, fmt.Errorf("art: excessive animation source copies")
	}
	ext, stem := path.Ext(name), p.base+"-"+strings.TrimSuffix(path.Base(name), path.Ext(name))
	target := assetops.UniqueFile(p.ed, path.Dir(stem), path.Base(stem), ext)
	for n := 2; p.planned[target]; n++ {
		target = assetops.UniqueFile(p.ed, path.Dir(stem), fmt.Sprintf("%s-%d", path.Base(stem), n), ext)
	}
	meta, err := p.ed.Pipeline.NewSourceMeta(target)
	if err != nil {
		return id, err
	}
	p.planned[target], p.planned[target+pipeline.MetaSuffix] = true, true
	p.edits = append(p.edits, pipeline.SourceEdit{Path: target, After: aetext.Write(node)}, pipeline.SourceEdit{Path: target + pipeline.MetaSuffix, After: pipeline.WriteMeta(meta)})
	p.ids[id] = meta.ID
	return meta.ID, nil
}
