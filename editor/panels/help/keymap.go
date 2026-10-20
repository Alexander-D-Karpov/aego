package help

import (
	"strings"

	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/services"
	"aego/ui/widgets"
)

const KeymapID = "keymap"

type kmRow struct {
	header bool
	name   string
	keys   string
}

var editorKeys = []kmRow{
	{name: "Completion", keys: "Ctrl+Space"},
	{name: "Parameter info", keys: "Ctrl+P"},
	{name: "Quick documentation", keys: "Ctrl+Q"},
	{name: "Open documentation", keys: "F1"},
	{name: "Go to declaration", keys: "F12 · Ctrl+B · Ctrl+Click"},
	{name: "Peek definition", keys: "Ctrl+Shift+I"},
	{name: "Find usages", keys: "Shift+F12 · Alt+F7"},
	{name: "File structure", keys: "Ctrl+F12"},
	{name: "Back / forward", keys: "Ctrl+Alt+Left / Right"},
	{name: "Next / previous error", keys: "F2 / Shift+F2"},
	{name: "Find / replace in file", keys: "Ctrl+F / Ctrl+H"},
	{name: "Find in scripts", keys: "Ctrl+Shift+F"},
	{name: "Toggle comment", keys: "Ctrl+/"},
	{name: "Indent / outdent", keys: "Tab / Shift+Tab"},
	{name: "Duplicate line", keys: "Ctrl+D"},
	{name: "Delete line", keys: "Ctrl+Shift+K"},
	{name: "Move line", keys: "Alt+Up / Down"},
	{name: "Expand / shrink selection", keys: "Ctrl+W / Ctrl+Shift+W"},
	{name: "Split right / down", keys: "Ctrl+\\ / Ctrl+Shift+\\"},
	{name: "Save", keys: "Ctrl+S"},
	{name: "Close tab", keys: "Ctrl+F4"},
	{name: "Float a panel", keys: "Shift+drag a tab · right-click a tab"},
}

type Keymap struct {
	cmds   *command.Registry
	query  string
	scroll float32
	rows   []kmRow
}

func NewKeymap(cmds *command.Registry) *Keymap {
	return &Keymap{cmds: cmds}
}

func (k *Keymap) ID() string {
	return KeymapID
}

func (k *Keymap) Title() string {
	return "Keymap"
}

func (k *Keymap) build() {
	q := strings.ToLower(strings.TrimSpace(k.query))
	match := func(r kmRow) bool {
		return q == "" || strings.Contains(strings.ToLower(r.name+" "+r.keys), q)
	}
	k.rows = k.rows[:0]
	section := func(title string, rows []kmRow) {
		start := len(k.rows)
		k.rows = append(k.rows, kmRow{header: true, name: title})
		for _, r := range rows {
			if match(r) {
				k.rows = append(k.rows, r)
			}
		}
		if len(k.rows) == start+1 {
			k.rows = k.rows[:start]
		}
	}
	section("Code editor", editorKeys)
	byCat := map[string][]kmRow{}
	var cats []string
	for _, d := range k.cmds.All() {
		keys := d.KeysText()
		if keys == "" {
			continue
		}
		cat := d.Category
		if cat == "" {
			cat = "Panels"
		}
		if _, ok := byCat[cat]; !ok {
			cats = append(cats, cat)
		}
		byCat[cat] = append(byCat[cat], kmRow{name: d.Name, keys: keys})
	}
	for _, c := range cats {
		section(c, byCat[c])
	}
}

func (k *Keymap) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	bar := ui.Cursor().Next(0)
	widgets.SearchField(ui, ui.ID("search"), bar, &k.query)
	k.build()
	top := bar.Y + bar.H + th.Spacing
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	widgets.VirtualList(ui, ui.ID("list"), list, len(k.rows), th.RowHeight, &k.scroll, func(i int, row mathx.Rect) {
		kr := k.rows[i]
		ty := ui.TextY(row)
		if kr.header {
			ui.Text(kr.name, mathx.Vec2{X: row.X + th.Padding, Y: ty}, th.Accent)
			return
		}
		ui.Text(kr.name, mathx.Vec2{X: row.X + th.Padding + th.Indent, Y: ty}, th.Text)
		ui.Text(kr.keys, mathx.Vec2{X: row.X + max(row.W*0.5, 220), Y: ty}, th.TextDim)
	})
}
