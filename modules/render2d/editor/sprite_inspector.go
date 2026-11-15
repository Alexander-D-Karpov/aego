package editor

import (
	"path"
	"strings"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/ext"
	"aego/editor/ext/inspect"
	"aego/engine/i18n"
	"aego/modules/render2d"
	"aego/scene"
	"aego/ui/widgets"
)

// A SpriteRenderer keeps its existing AssetRef. Opening the source changes no
// scene property: the document manager resolves frame IDs to their owner.
func spriteInspector(ed *context.Editor, nodes []ids.NodeID, defaults func()) {
	defaults()
	if ed == nil || ed.Docs == nil || ed.Doc() == nil || ed.Pipeline == nil || ed.UI == nil || len(nodes) != 1 {
		return
	}
	s := ed.Doc().Scene()
	sprite, ok := scene.Get[render2d.SpriteRenderer](s, s.Handle(nodes[0]))
	if !ok || sprite.Sprite.AssetRefID().IsZero() {
		return
	}
	id := sprite.Sprite.AssetRefID()
	source, ok := ed.SourcePathForAsset(id)
	if !ok {
		return
	}
	label := i18n.Tr("editor.art.open_sprite_source", "Open Sprite Source")
	tip := i18n.Tr("editor.art.prepared_sprite", "Open the image or sprite in the asset viewer.")
	if strings.EqualFold(path.Ext(source), ".aeart") {
		label = i18n.Tr("editor.art.edit_sprite", "Edit Sprite")
		tip = i18n.Tr("editor.art.shared_sprite", "This is a shared asset; edits affect every object that uses it.")
	}
	editable := ed.EditSourceArt != nil && ed.CanEditSourceArt != nil && ed.CanEditSourceArt(id)
	canLink := editable && !strings.EqualFold(path.Ext(source), ".aeart")
	canDuplicate := editable && !ed.Doc().IsPlay()
	canFindUses := ed.FindAssetUses != nil
	clicked := false
	if canLink || canDuplicate || canFindUses {
		// Keep viewing, editing and object-local duplication in one compact row.
		row := ed.UI.Cursor().Next(0)
		count := 1
		if canLink {
			count++
		}
		if canDuplicate {
			count++
		}
		gap := ed.UI.Theme.Spacing
		available := row.W
		refWidth := float32(0)
		if canFindUses {
			refWidth = min(row.H, max(0, row.W))
			available = max(0, available-refWidth-gap)
		}
		width := max(0, (available-float32(count-1)*gap)/float32(count))
		cell := func(index int) mathx.Rect {
			return mathx.Rect{X: row.X + float32(index)*(width+gap), Y: row.Y, W: width, H: row.H}
		}
		clicked = widgets.ButtonRect(ed.UI, ed.UI.ID(label), cell(0), widgets.Elide(ed.UI, label, max(0, width-8)))
		index := 1
		if canLink {
			editLabel := i18n.Tr("editor.art.edit_source", "Edit Source Art")
			editID := ed.UI.ID("sprite.edit_source_art")
			if widgets.ButtonRect(ed.UI, editID, cell(index), widgets.Elide(ed.UI, editLabel, max(0, width-8))) {
				if err := ed.EditSourceArt(id); err != nil {
					ed.Notify(context.LevelError, err.Error())
				}
			}
			widgets.Tooltip(ed.UI, editID, editLabel)
			index++
		}
		if canDuplicate {
			copyLabel := i18n.Tr("editor.art.duplicate_for_object", "Duplicate for This Object")
			copyID := ed.UI.ID("sprite.duplicate_source_art")
			if widgets.ButtonRect(ed.UI, copyID, cell(index), widgets.Elide(ed.UI, copyLabel, max(0, width-8))) {
				if err := askDuplicateSpriteSource(ed, nodes[0], id); err != nil {
					ed.Notify(context.LevelError, err.Error())
				}
			}
			widgets.Tooltip(ed.UI, copyID, copyLabel)
		}
		if canFindUses {
			refID := ed.UI.ID("sprite.find_uses")
			ref := mathx.Rect{X: row.X + row.W - refWidth, Y: row.Y, W: refWidth, H: row.H}
			if ref.W >= 8 && widgets.IconButtonRect(ed.UI, refID, ref, "panel.references") {
				ed.FindAssetUses(id)
			}
			widgets.Tooltip(ed.UI, refID, i18n.Tr("editor.assets.find_references", "Find References"))
		}
	} else {
		clicked = widgets.Button(ed.UI, label)
	}
	widgets.Tooltip(ed.UI, ed.UI.ID(label), tip)
	if clicked {
		if _, err := ed.OpenAsset(id); err != nil {
			ed.Notify(context.LevelError, err.Error())
		}
	}
}

func spriteInspectorHooks(r *ext.Registry) error {
	return r.Inspector("engine.SpriteRenderer", inspect.CustomFunc(spriteInspector))
}
