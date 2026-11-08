package shell

import (
	"aego/editor/core/workspace"
	"aego/engine/i18n"
	"aego/ui/dock"
)

// Factory-only composition. No revision bump: saved user layouts remain intact.
// Asset Timeline is a separate panel from the existing scene Animation panel.
func authoringWorkspaces() []workspace.Preset {
	left, center, bottom, right := workspace.SlotLeft, workspace.SlotCenter, workspace.SlotBottom, workspace.SlotRight
	artScope := map[string]workspace.Slot{"render2d.art": center}
	return []workspace.Preset{
		{ID: workspace.Animation, Title: i18n.Tr("editor.shell.animation", "Animation"), Order: 21, Explicit: true, Scopes: artScope,
			Slots: map[string]workspace.Slot{workspace.Assets: left, "art.preview": center, "art.properties": right, "art.timeline": bottom, workspace.Scene: center},
			Front: []string{"art.timeline", "art.properties"}},
		{ID: workspace.VFX, Title: i18n.Tr("editor.shell.vfx", "VFX"), Order: 22, Explicit: true, Scopes: map[string]workspace.Slot{"render2d.art": center, "particles.preset": right},
			Slots:  map[string]workspace.Slot{workspace.Assets: left, workspace.Scene: center, workspace.Game: center, workspace.Inspector: right, "art.properties": right, "art.preview": center, "art.timeline": bottom},
			Splits: []workspace.PanelSplit{{Panel: "art.preview", Anchor: workspace.Scene, Side: dock.SideRight, Ratio: .5}},
			Front:  []string{workspace.Scene, "art.timeline"}},
		{ID: workspace.Default, Title: i18n.Tr("editor.workspace.default", "Default"), Order: 40, Explicit: true, Scopes: artScope,
			Slots: map[string]workspace.Slot{workspace.Hierarchy: left, workspace.Assets: workspace.SlotBrowser, workspace.Scene: center, workspace.Game: center, workspace.Inspector: right, workspace.Problems: bottom, workspace.Console: bottom},
			Front: []string{workspace.Scene, workspace.Problems}},
	}
}
