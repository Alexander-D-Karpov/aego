package help

import (
	"strings"

	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/services"
	"aego/engine/i18n"
	"aego/ui/widgets"
)

const KeymapID = "keymap"

type kmRow struct {
	header bool
	name   string
	keys   string
}

func editorKeys() []kmRow {
	return []kmRow{
		{name: i18n.Tr("editor.keymap.completion", "Completion"), keys: "Ctrl+Space"},
		{name: i18n.Tr("editor.keymap.parameter_info", "Parameter info"), keys: "Ctrl+P"},
		{name: i18n.Tr("editor.keymap.quick_documentation", "Quick documentation"), keys: "Ctrl+Q"},
		{name: i18n.Tr("editor.keymap.open_documentation", "Open documentation"), keys: "F1"},
		{name: i18n.Tr("editor.keymap.go_to_declaration", "Go to declaration"), keys: "F12 · Ctrl+B · Ctrl+Click"},
		{name: i18n.Tr("editor.keymap.peek_definition", "Peek definition"), keys: "Ctrl+Shift+I"},
		{name: i18n.Tr("editor.keymap.find_usages", "Find usages"), keys: "Shift+F12 · Alt+F7"},
		{name: i18n.Tr("editor.keymap.file_structure", "File structure"), keys: "Ctrl+F12"},
		{name: i18n.Tr("editor.keymap.back_forward", "Back / forward"), keys: "Ctrl+Alt+Left / Right"},
		{name: i18n.Tr("editor.keymap.next_previous_error", "Next / previous error"), keys: "F2 / Shift+F2"},
		{name: i18n.Tr("editor.keymap.find_replace_in_file", "Find / replace in file"), keys: "Ctrl+F / Ctrl+H"},
		{name: i18n.Tr("editor.keymap.find_in_scripts", "Find in scripts"), keys: "Ctrl+Shift+F"},
		{name: i18n.Tr("editor.keymap.toggle_comment", "Toggle comment"), keys: "Ctrl+/"},
		{name: i18n.Tr("editor.keymap.indent_outdent", "Indent / outdent"), keys: "Tab / Shift+Tab"},
		{name: i18n.Tr("editor.keymap.duplicate_line", "Duplicate line"), keys: "Ctrl+D"},
		{name: i18n.Tr("editor.keymap.delete_line", "Delete line"), keys: "Ctrl+Shift+K"},
		{name: i18n.Tr("editor.keymap.move_line", "Move line"), keys: "Alt+Up / Down"},
		{name: i18n.Tr("editor.keymap.expand_shrink_selection", "Expand / shrink selection"), keys: "Ctrl+W / Ctrl+Shift+W"},
		{name: i18n.Tr("editor.keymap.split_right_down", "Split right / down"), keys: "Ctrl+\\ / Ctrl+Shift+\\"},
		{name: i18n.Tr("editor.keymap.save", "Save"), keys: "Ctrl+S"},
		{name: i18n.Tr("editor.keymap.close_tab", "Close tab"), keys: "Ctrl+F4"},
		{name: i18n.Tr("editor.keymap.float_a_panel", "Float a panel"), keys: "Shift+drag a tab · right-click a tab"},
	}
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
	return i18n.Tr("editor.keymap.title", "Keymap")
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
	section(i18n.Tr("editor.help.code_editor", "Code editor"), editorKeys())
	byCat := map[string][]kmRow{}
	var cats []string
	for _, d := range k.cmds.All() {
		keys := d.KeysText()
		if keys == "" {
			continue
		}
		cat := d.Category
		if cat == "" {
			cat = i18n.Tr("editor.menu.panels", "Panels")
		}
		if _, ok := byCat[cat]; !ok {
			cats = append(cats, cat)
		}
		byCat[cat] = append(byCat[cat], kmRow{name: d.Title(), keys: keys})
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
