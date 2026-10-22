package site

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var kindIcons = map[string]string{
	KindPage:     "help.book",
	KindModule:   "cat.node",
	KindFunction: "code.brackets",
	KindType:     "code.block",
	KindField:    "code.hints",
	KindMethod:   "code.brackets",
	KindSignal:   "panel.events",
	KindCode:     "log.error",
	KindConst:    "code.hints",
	KindEnum:     "code.hints",
}

var (
	svgOpen  = regexp.MustCompile(`(?s)<svg[^>]*?viewBox="([^"]+)"[^>]*>(.*)</svg>`)
	svgWhite = regexp.MustCompile(`(?i)(fill|stroke)="#(fff|ffffff|white)"`)
)

type iconSet struct {
	names  map[string]bool
	sprite string
}

func loadIcons(dirs []string) iconSet {
	set := iconSet{names: map[string]bool{}}
	body := map[string]string{}
	for _, d := range dirs {
		files, _ := filepath.Glob(filepath.Join(d, "*.svg"))
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			m := svgOpen.FindStringSubmatch(string(data))
			if m == nil {
				continue
			}
			name := strings.TrimSuffix(filepath.Base(f), ".svg")
			inner := svgWhite.ReplaceAllString(m[2], `$1="currentColor"`)
			body[name] = `<symbol id="` + name + `" viewBox="` + m[1] + `">` + inner + "</symbol>"
		}
	}
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg">`)
	for _, k := range keys {
		set.names[k] = true
		b.WriteString(body[k])
	}
	b.WriteString("</svg>\n")
	set.sprite = b.String()
	return set
}

func (b *builder) icon(root, name, cls string) string {
	if name == "" {
		return ""
	}
	if !b.icons.names[name] {
		if len(b.icons.names) > 0 && !b.missing[name] {
			b.missing[name] = true
			b.report.Broken = append(b.report.Broken, "icon "+name+" does not exist")
		}
		return ""
	}
	if cls == "" {
		cls = "ic"
	} else {
		cls = "ic " + cls
	}
	return `<svg width="16" height="16" class="` + cls + `" aria-hidden="true"><use href="` + root + `assets/icons.svg?v=` + b.assetV + `#` + name + `"/></svg>`
}

// refIcons gives script packages and project settings sections the editor icon of what they cover.
var refIcons = map[string]string{
	"aego/builtin": "cat.node", "events": "panel.events", "aego/action": "panel.tasks", "aego/audio": "cat.audio",
	"aego/camera": "cat.camera", "aego/curve": "ui.curve", "aego/data": "file.data", "aego/debug": "panel.debug_draw",
	"aego/input": "panel.input_devices", "aego/log": "panel.console", "aego/math": "cat.transform", "aego/physics": "cat.physics",
	"aego/rand": "shape.dice", "aego/save": "panel.saves", "aego/scene": "file.scene", "aego/settings": "cat.settings",
	"aego/testing": "status.ok", "aego/time": "play.speed", "aego/timer": "file.autosave", "aego/tween": "vec.velocity",
	"section/engine": "panel.settings", "section/input": "panel.input_devices", "section/render2d": "cat.rendering",
	"section/physics": "cat.physics", "section/debug": "panel.debug_draw", "section/player": "play.start", "section/audio": "cat.audio",
}

func (b *builder) entryIcon(e *Entry) string {
	if e.Icon != "" {
		return e.Icon
	}
	if ic, ok := refIcons[e.ID]; ok {
		return ic
	}
	if e.Kind == KindField && e.Field != nil {
		return fieldIcon(e.Title, e.Field.Type)
	}
	if e.Kind == KindType && e.Parent != "" {
		if ic, ok := refIcons[e.Parent]; ok {
			return ic
		}
	}
	if e.Kind == KindModule && strings.HasPrefix(e.ID, "aego/") {
		return "code.block"
	}
	return kindIcons[e.Kind]
}

func fieldIcon(name, typ string) string {
	n := strings.ToLower(name)
	t := strings.ToLower(typ)
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
