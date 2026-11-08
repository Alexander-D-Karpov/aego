package editor

import (
	"path"
	"strings"

	"aego/core/ids"
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
	clicked := widgets.Button(ed.UI, label)
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
