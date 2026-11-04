package editor

import (
	"slices"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/ui/core"
	"aego/ui/widgets"
	"aego/ui/widgets/palette"
)

type imageChannelPicker struct {
	palette *palette.Palette
	refs    []ids.AssetID
	frame   ids.AssetID
}

// Reuse the searchable keyboard palette used by Create/Add Component. A sheet
// can have hundreds of compatible map frames; a growing unscrollable menu is
// not an asset picker. This is presentation state, not a second document model.
func (v *imageState) chooseChannel(ui *core.Context, field mathx.Rect, label string, labels []string, refs []ids.AssetID, current int, frame ids.AssetID) (ids.AssetID, bool, bool) {
	if v.pickers == nil {
		v.pickers = map[core.ID]*imageChannelPicker{}
	}
	id := ui.ID("image.channel.palette")
	p := v.pickers[id]
	if p == nil {
		p = &imageChannelPicker{palette: palette.New(id)}
		p.palette.Describe = func(ui *core.Context, r mathx.Rect, index int) {
			if index < 0 || index >= len(p.palette.Items()) {
				return
			}
			ui.Region(r)
			defer ui.PopCursor()
			widgets.LabelWrapped(ui, p.palette.Item(index).Label, false)
			if index < len(p.refs) && !p.refs[index].IsZero() {
				widgets.LabelWrapped(ui, p.refs[index].String(), false)
			}
		}
		v.pickers[id] = p
	}
	buttonID := ui.ID("choose")
	if widgets.ButtonRect(ui, buttonID, field, widgets.Elide(ui, labels[current], max(0, field.W-12))) {
		p.refs = slices.Clone(refs)
		p.frame = frame
		items := make([]palette.Item, len(labels))
		for i, text := range labels {
			items[i] = palette.Item{Key: refs[i].String(), Label: text, Category: label, Icon: "file.texture"}
		}
		p.palette.Title = i18n.Tr("editor.image.choose_channel", "Choose {channel}", i18n.Str("channel", label))
		p.palette.Hint = i18n.Tr("editor.image.picker_hint", "Type to search · arrows to choose · Enter to assign · Esc to cancel")
		p.palette.SetItems(items)
		p.palette.Show(ui)
	}
	widgets.Tooltip(ui, buttonID, labels[current])
	if !p.palette.Active() {
		return ids.AssetID{}, false, false
	}
	if p.frame != frame {
		p.palette.Hide(ui)
		return ids.AssetID{}, false, false
	}
	size := ui.Size()
	w, h := min(float32(720), max(240, size.X-32)), min(float32(520), max(200, size.Y-32))
	result := p.palette.Draw(ui, mathx.Rect{X: (size.X - w) / 2, Y: (size.Y - h) / 2, W: w, H: h})
	if result.Pick >= 0 && result.Pick < len(p.refs) {
		ref, inherit := p.refs[result.Pick], result.Pick == 0
		p.palette.Hide(ui)
		return ref, inherit, true
	}
	if result.Close {
		p.palette.Hide(ui)
	}
	return ids.AssetID{}, false, false
}
