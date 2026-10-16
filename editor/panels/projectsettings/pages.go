package projectsettings

import (
	"strconv"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/platform/input"
	"aego/project"
	"aego/ui/core"
	"aego/ui/widgets"
)

type page struct {
	id      string
	title   string
	order   int
	section string
	project func(*core.Context, *project.Project) bool
	draw    func(any) bool
	drawUI  func(*core.Context, any) bool
}

func (p *Panel) builtin() []page {
	return []page{
		{id: "project", title: "Project", order: 0, project: p.drawProject},
		{id: "input", title: "Input", order: 200, project: p.drawInput},
	}
}

func intField(ui *core.Context, label string, v *int, lo float64) bool {
	f := float64(*v)
	e := widgets.DragNumber(ui, ui.ID(label), widgets.Field(ui, label), &f, widgets.NumberOpts{Int: true, Speed: 1, Min: lo, HasMin: true})
	if e.Changed {
		*v = int(f)
	}
	return e.Changed
}

func textField(ui *core.Context, label string, v *string) bool {
	_, committed := widgets.TextFieldRect(ui, ui.ID(label), widgets.Field(ui, label), v, false)
	return committed
}

func (p *Panel) drawProject(ui *core.Context, pr *project.Project) bool {
	changed := textField(ui, "Name", &pr.Name)
	ui.TextIn(widgets.Field(ui, "Module"), pr.Module, ui.Theme.TextDim, core.AlignLeft)
	if p.d.Pipeline != nil {
		var names []string
		var list []ids.AssetID
		cur := -1
		for _, a := range p.d.Pipeline.Assets() {
			if a.Kind != asset.ArtScene {
				continue
			}
			if a.ID == pr.MainScene {
				cur = len(names)
			}
			names = append(names, a.Path)
			list = append(list, a.ID)
		}
		if widgets.Combo(ui, ui.ID("main"), widgets.Field(ui, "Startup Scene"), &cur, names, false) && cur >= 0 {
			pr.MainScene = list[cur]
			changed = true
		}
	}
	space := 0
	if pr.DefaultSpace == project.Space3D {
		space = 1
	}
	if widgets.Combo(ui, ui.ID("space"), widgets.Field(ui, "Default Space"), &space, []string{"2D", "3D"}, false) {
		pr.DefaultSpace = project.Space2D
		if space == 1 {
			pr.DefaultSpace = project.Space3D
		}
		changed = true
	}
	widgets.Separator(ui)
	widgets.Label(ui, "Window")
	changed = intField(ui, "Width", &pr.Window.Width, 64) || changed
	changed = intField(ui, "Height", &pr.Window.Height, 64) || changed
	changed = textField(ui, "Title", &pr.Window.Title) || changed
	return changed
}

func (p *Panel) keyField(ui *core.Context, id core.ID, r mathx.Rect, k *input.Scancode) bool {
	text, _ := input.KeyName(*k)
	_, committed := widgets.TextFieldRect(ui, id, r, &text, false)
	if !committed {
		return false
	}
	if text == "" {
		*k = input.KeyUnknown
		return true
	}
	parsed, ok := input.ParseKey(text)
	if !ok {
		p.d.Notices.Notify(context.LevelWarn, "unknown key "+text)
		return false
	}
	*k = parsed
	return true
}

func (p *Panel) drawInput(ui *core.Context, pr *project.Project) bool {
	th := &ui.Theme
	changed := false
	head := ui.Cursor().Next(0)
	cols := [...]string{"Axis", "Negative", "Positive", "Alt −", "Alt +"}
	w := (head.W - head.H - 4) / 5
	for i, c := range cols {
		ui.Text(c, mathx.Vec2{X: head.X + float32(i)*w, Y: ui.TextY(head)}, th.TextDim)
	}
	remove := -1
	for i := range pr.Input.Axes {
		a := &pr.Input.Axes[i]
		ui.PushIDInt(i)
		row := ui.Cursor().Next(0)
		cell := func(j int) mathx.Rect {
			return mathx.Rect{X: row.X + float32(j)*w, Y: row.Y, W: w - 4, H: row.H}
		}
		if _, ok := widgets.TextFieldRect(ui, ui.ID("name"), cell(0), &a.Name, false); ok {
			changed = true
		}
		for j, k := range [...]*input.Scancode{&a.Negative, &a.Positive, &a.AltNegative, &a.AltPositive} {
			if p.keyField(ui, ui.IDInt(10+j), cell(j+1), k) {
				changed = true
			}
		}
		if widgets.ButtonRect(ui, ui.ID("remove"), mathx.Rect{X: row.X + row.W - row.H, Y: row.Y, W: row.H, H: row.H}, "−") {
			remove = i
		}
		ui.PopID()
	}
	if remove >= 0 {
		pr.Input.Axes = append(pr.Input.Axes[:remove], pr.Input.Axes[remove+1:]...)
		changed = true
	}
	if widgets.Button(ui, "Add Axis") {
		pr.Input.Axes = append(pr.Input.Axes, input.Axis{Name: "axis" + strconv.Itoa(len(pr.Input.Axes)+1)})
		changed = true
	}
	widgets.LabelWrapped(ui, "Scripts read axes with input.Axis(\"name\"). Key names: A…Z, Digit0…Digit9, Left, Right, Up, Down, Space, Enter, LShift…", true)
	return changed
}
