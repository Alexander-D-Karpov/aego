//go:build aego_editor

package native

import (
	"reflect"
	"strconv"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/platform/input"
	"aego/scene"
	"aego/schema"
	"aego/sdk"
	"aego/ui/widgets"
)

func registerEditor(r *sdk.Registry) {
	sdk.EditorHook(r, func(e *sdk.EditorRegistry) error {
		ed := e.Editor()
		if err := e.Inspector("game.Health", sdk.InspectorFunc(healthInspector)); err != nil {
			return err
		}
		if err := e.Panel(sdk.PanelDesc{Panel: &questPanel{ed: ed}, Near: sdk.PanelInspector}); err != nil {
			return err
		}
		return e.Command(sdk.CommandDesc{
			ID:       "game.heal_all",
			Name:     "Heal All",
			Category: "Game",
			Shortcut: command.Key(input.KeyH, command.ModPrimary|command.ModShift),
			Enabled:  func() bool { return ed.Doc() != nil },
			Run:      func() error { return heal(ed, healthNodes(ed)) },
		})
	})
}

func healthField(ed *sdk.Editor) (schema.TypeID, schema.FieldID, bool) {
	td, ok := ed.Types.Schema().ByGoType(reflect.TypeFor[Health]())
	if !ok {
		return 0, 0, false
	}
	fd, ok := td.Field("current")
	if !ok {
		return 0, 0, false
	}
	return td.ID, fd.ID, true
}

func healthNodes(ed *sdk.Editor) []ids.NodeID {
	d := ed.Doc()
	if d == nil {
		return nil
	}
	s := d.Scene()
	var out []ids.NodeID
	scene.Query1[Health](s).Each(func(h scene.NodeHandle, _ *Health) {
		out = append(out, s.ID(h))
	})
	return out
}

func heal(ed *sdk.Editor, nodes []ids.NodeID) error {
	d := ed.Doc()
	if d == nil || len(nodes) == 0 {
		return nil
	}
	t, f, ok := healthField(ed)
	if !ok {
		return nil
	}
	s := d.Scene()
	tx := d.History.Begin("Heal")
	for _, n := range nodes {
		hp, ok := scene.Get[Health](s, s.Handle(n))
		if !ok {
			continue
		}
		if err := d.Props.Set(tx, []ids.NodeID{n}, t, f, schema.Float32(hp.Max)); err != nil {
			tx.Abort()
			return err
		}
	}
	tx.Commit()
	return nil
}

func healthInspector(ed *sdk.Editor, nodes []ids.NodeID, defaultFields func()) {
	defaultFields()
	ui := ed.UI
	th := &ui.Theme
	if d := ed.Doc(); d != nil && len(nodes) > 0 {
		s := d.Scene()
		if hp, ok := scene.Get[Health](s, s.Handle(nodes[0])); ok && hp.Max > 0 {
			bar := ui.Cursor().Next(6)
			ui.Rect(bar, th.Widget)
			ui.Rect(mathx.Rect{X: bar.X, Y: bar.Y, W: bar.W * mathx.Clamp(hp.Current/hp.Max, 0, 1), H: bar.H}, th.Ok)
		}
	}
	if widgets.Button(ui, "Heal") {
		if err := heal(ed, nodes); err != nil {
			ed.Notify(context.LevelError, err.Error())
		}
	}
}

type questPanel struct {
	ed *sdk.Editor
}

func (*questPanel) ID() string {
	return "game.quest"
}

func (*questPanel) Title() string {
	return "Quest"
}

func (q *questPanel) Draw(f *sdk.Frame, r mathx.Rect) {
	ui := f.UI
	ed := q.ed
	d := ed.Doc()
	if d == nil {
		widgets.LabelDim(ui, "No scene open")
		return
	}
	s := d.Scene()
	i := 0
	scene.Query1[Health](s).Each(func(h scene.NodeHandle, hp *Health) {
		id := s.ID(h)
		label := s.Name(h) + "  " + strconv.Itoa(int(hp.Current)) + " / " + strconv.Itoa(int(hp.Max))
		if widgets.Selectable(ui, ui.IDInt(i), label, ed.IsSelected(id)) {
			ed.Select(id)
		}
		i++
	})
	if i == 0 {
		widgets.LabelDim(ui, "No node has Health yet. Create > Enemy adds one.")
	}
}
