package editor

import (
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/modules/particles"
	"aego/platform/input"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/ui/widgets"
	"errors"
	"path"
)

type presetPanel struct {
	scroll, height float32
	bar            widgets.ScrollState
	a              *adapter
	d              *document.AssetDocument
	v              transport
	surf           surface
	grad           GradientEditor
	rev            uint64
	generation     uint32
	now            float64
	closed         bool
	gesture        string
}

func (p *adapter) panel(d *document.AssetDocument) workspace.Panel {
	if x, ok := d.View.(*presetPanel); ok && !x.closed {
		return x
	}
	x := &presetPanel{a: p, d: d}
	d.View = x
	p.panels[d.ID] = x
	if d.Source != nil {
		d.Source.ExplicitSave = true
		ed := p.ed
		d.Source.ValidateSave = func(data []byte) error {
			c, err := particles.ReadPreset(ed.Types, resolver(ed), data)
			if err != nil {
				return err
			}
			return particles.CheckResources(&c, resolver(ed))
		}
	}
	return x
}
func (x *presetPanel) ID() string                        { return "particles.preset/" + x.d.Asset.String() }
func (x *presetPanel) Title() string                     { return path.Base(x.d.Path) }
func (x *presetPanel) Document() *document.AssetDocument { return x.d }
func (x *presetPanel) Dirty() bool                       { return x.d.Dirty() }
func (x *presetPanel) Scope() string                     { return "particles.preset" }
func (x *presetPanel) Icon() string                      { return "file.preset" }
func (x *presetPanel) save() bool {

	if err := x.a.ed.Docs.SaveAsset(x.d); err != nil {
		x.a.ed.Notify(context.LevelError, err.Error())
		return false
	}
	return true
}
func (x *presetPanel) dispose() {
	if x.closed {
		return
	}
	x.closed = true
	x.v.close()
	x.surf.close()
	x.d.View = nil
	delete(x.a.panels, x.d.ID)
}
func (x *presetPanel) close(force bool) {
	if x.closed {
		return
	}
	if err := x.a.ed.Docs.CloseAsset(x.d.Asset, force); err != nil {
		x.a.ed.Notify(context.LevelError, err.Error())
		return
	}
	if !x.closed {
		x.dispose()
		x.a.reg.ClosePanel(x.ID())
	}
}
func (x *presetPanel) RequestClose() {
	if !x.Dirty() {
		x.close(false)
		return
	}
	x.a.ed.Ask(context.Question{Title: i18n.Tr("editor.particles.close", "Unsaved particle preset"), Message: x.d.Path, Buttons: []string{i18n.Tr("editor.particles.save", "Save preset"), i18n.Tr("editor.particles.discard", "Discard changes"), i18n.Tr("editor.particles.cancel", "Cancel")}, Then: func(i int) {
		if i == 0 && x.save() {
			x.close(false)
		} else if i == 1 {
			x.close(true)
		}
	}})
}
func (x *presetPanel) change(k string, v aetext.Value, cont bool) error {
	if x.d.Source == nil {
		return errors.New("preset source unavailable")
	}
	src := x.d.Source.Code
	n, c, err := particleNode(src.Bytes(), x.d.Path)
	if err != nil {
		return err
	}
	c.SetField(k, v)
	data := aetext.Write(n)
	if _, err := particles.ReadPreset(x.a.ed.Types, resolver(x.a.ed), data); err != nil {
		return err
	}
	src.ReplaceSnapshot(data, cont)
	return nil
}
func (x *presetPanel) Draw(f *services.Frame, area mathx.Rect) {
	ed, ui := x.a.ed, f.UI
	ui.PushID(x.ID())
	defer ui.PopID()
	ui.PushClip(area)
	defer ui.PopClip()
	ui.Region(area)
	defer ui.PopCursor()
	cur := ui.Cursor()
	if ui.MouseOver(ui.ID("fx.scroll"), area) {
		x.scroll -= ui.ScrollY(ui.Theme.RowHeight * 3)
	}
	x.scroll = mathx.Clamp(x.scroll, 0, max(0, x.height-area.H))
	cur.Y -= x.scroll
	cur.W -= 9
	defer func() {
		x.height = cur.Y + x.scroll - area.Y
		widgets.ScrollBar(ui, ui.ID("fx.bar"), mathx.Rect{X: area.X + area.W - 7, Y: area.Y, W: 7, H: area.H}, &x.scroll, x.height, area.H, &x.bar)
	}()
	if x.d.Source == nil {
		widgets.LabelDim(ui, i18n.Tr("editor.particles.no_source", "Preset source is unavailable"))
		return
	}
	code := x.d.Source.Code
	first := x.v.sim == nil
	var gen uint32
	if ed.Loader != nil {
		gen = ed.Loader.Generation()
	}
	if first || x.rev != code.Rev() || x.generation != gen {
		x.v.init(ed)
		c, err := particles.ReadPreset(ed.Types, resolver(ed), code.Bytes())
		if err != nil {
			x.v.bad = err.Error()
		} else {
			x.v.configure(ed, c, first)
		}
		x.rev = code.Rev()
		x.generation = gen
	}
	if !x.v.paused && x.now != 0 {
		x.v.sim.Step(min(.1, max(0, f.Now-x.now))*float64(x.v.speed), mathx.Mat3Identity())
	}
	x.now = f.Now
	if err := x.surf.prepare(ed, 192); err != nil {
		widgets.LabelDim(ui, err.Error())
	} else {
		t := x.surf.target
		ed.OnRender(func() {
			if !x.closed && x.surf.target == t {
				x.surf.render(&x.v, false)
			}
		})
		rect := ui.Cursor().Next(160)
		rect.W = min(160, rect.W)
		ui.List().Texture(t.Region(), rect, mathx.White)
	}
	if widgets.Button(ui, i18n.Tr("editor.particles.save", "Save preset")) {
		x.save()
	}
	if widgets.Button(ui, i18n.Tr("editor.particles.undo", "Undo preset edit")) {
		code.Undo()
	}
	if widgets.Button(ui, i18n.Tr("editor.particles.redo", "Redo preset edit")) {
		code.Redo()
	}
	if ed.OpenScript != nil && widgets.Button(ui, i18n.Tr("editor.particles.source", "Edit preset source")) {
		ed.OpenScript(x.d.Path, 1)
	}
	x.v.buttons(ed)
	for _, field := range []struct {
		name string
		v    float32
	}{{"rate", x.v.cfg.Rate}, {"duration", x.v.cfg.Duration}, {"radius", x.v.cfg.Radius}, {"direction", x.v.cfg.Direction}, {"spread", x.v.cfg.Spread}, {"drag", x.v.cfg.Drag}} {
		val := field.v
		if widgets.DragFloat(ui, ui.ID(field.name), field.name, &val, .02) {
			if err := x.change(field.name, aetext.Float32(val), x.gesture == field.name); err != nil {
				ed.Notify(context.LevelError, err.Error())
			}
			x.gesture = field.name
		}
	}
	if !ui.MouseDown(input.MouseLeft) {
		x.gesture = ""
	}
	x.grad.Draw(ui, x.v.cfg.ColorOverLife, func(keys []particles.ColorKey) {
		vs := make([]aetext.Value, len(keys))
		for i, k := range keys {
			c := k.Color
			vs[i] = aetext.Struct(aetext.Field{Name: "time", Value: aetext.Float32(k.Time)}, aetext.Field{Name: "color", Value: sceneio.WriteValue(ed.Types.Schema(), &schema.FieldDescriptor{Kind: schema.KindColor}, schema.Color(c))})
		}
		if err := x.change("colorOverLife", aetext.List(vs...), false); err != nil {
			ed.Notify(context.LevelError, err.Error())
		}
	})
}
