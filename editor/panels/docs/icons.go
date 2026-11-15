package docs

import (
	"strings"

	regdocs "aego/engine/docs"
	"aego/ui/core"
	"aego/ui/widgets"
)

var kindIcons = map[regdocs.Kind]string{
	regdocs.Page: "help.book", regdocs.Module: "cat.node", regdocs.Function: "code.brackets",
	regdocs.Type: "code.block", regdocs.Field: "code.hints", regdocs.Method: "code.brackets",
	regdocs.Signal: "panel.events", regdocs.Code: "log.error", regdocs.Const: "code.hints", regdocs.Enum: "code.hints",
}

func (p *Panel) entryIcon(e regdocs.Entry) string {
	if ic := p.reg.NavigationTaskIcon(e.ID); ic != "" {
		return ic
	}
	if e.Icon != "" {
		return e.Icon
	}
	if ic := p.reg.NavigationReferenceIcon(e.ID); ic != "" {
		return ic
	}
	if e.Kind == regdocs.Field && e.Field != nil {
		return fieldIcon(e.Title, e.Field.Type)
	}
	if e.Kind == regdocs.Type && e.Parent != "" {
		if ic := p.reg.NavigationReferenceIcon(e.Parent); ic != "" {
			return ic
		}
	}
	if e.Kind == regdocs.Module && strings.HasPrefix(e.ID, "aego/") {
		return "code.block"
	}
	if e.Kind == regdocs.Page && e.Parent != "" && e.Parent != e.ID && p.reg != nil {
		if parent, ok := p.reg.Get(e.Parent); ok {
			if ic := p.reg.NavigationTaskIcon(parent.ID); ic != "" {
				return ic
			}
			if parent.Icon != "" {
				return parent.Icon
			}
		}
	}
	return kindIcons[e.Kind]
}

func fieldIcon(name, typ string) string {
	n, t := strings.ToLower(name), strings.ToLower(typ)
	switch {
	case strings.Contains(n, "gravity"):
		return "vec.gravity"
	case strings.Contains(n, "angular"):
		return "vec.angular"
	case strings.Contains(n, "velocity"), strings.Contains(n, "speed"):
		return "vec.velocity"
	case strings.Contains(n, "force"), strings.Contains(n, "impulse"):
		return "vec.force"
	case strings.Contains(n, "rotation"), t == "quat":
		return "tool.rotate"
	case strings.Contains(n, "scale"), strings.Contains(n, "size"):
		return "tool.scale"
	case strings.Contains(n, "position"), strings.Contains(n, "offset"), strings.Contains(n, "anchor"):
		return "tool.move"
	case strings.Contains(t, "texture"), strings.Contains(n, "sprite"):
		return "file.texture"
	case strings.Contains(t, "mesh"):
		return "file.mesh"
	case strings.Contains(t, "prefab"):
		return "file.prefab"
	case strings.Contains(t, "scene"):
		return "file.scene"
	case strings.HasPrefix(t, "noderef"), strings.Contains(t, "node"):
		return "node.empty"
	case t == "bool":
		return "ui.check"
	case strings.Contains(n, "layer"), strings.Contains(n, "mask"):
		return "view.overlays"
	case strings.Contains(n, "fixed"):
		return "vec.fixed"
	}
	return "code.hints"
}

// The website's code.timer SVG is not in the shipped atlas. Prefer it when an
// atlas does provide it; otherwise use the already-baked time-scale symbol.
// Missing plugin icons and an absent atlas must never hide or replace labels.
func availableIcon(id string, has func(string) bool) string {
	if id != "" && has(id) {
		return id
	}
	if id == "code.timer" && has("play.speed") {
		return "play.speed"
	}
	if has("help.book") {
		return "help.book"
	}
	return ""
}

func treeIcon(ui *core.Context, width float32, depth int, id string) string {
	// Preserve room for the label when the dock or an indented row is narrow.
	if width-float32(depth)*ui.Theme.Indent-ui.Theme.Indent-4 < 48 {
		return ""
	}
	return availableIcon(id, func(name string) bool { return widgets.HasIcon(ui, name) })
}
