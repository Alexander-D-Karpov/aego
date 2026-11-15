package editor

import (
	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/editor/core/assetops"
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
	"fmt"
	"path"
	"slices"
	"strings"
)

// The particle runtime uses a constant frame rate. It cannot reproduce the
// per-frame durations or tags in an Art animation.
type drawingFrames struct {
	ed                 *context.Editor
	doc                *document.AssetDocument
	model              *artedit.Document
	path               string
	ids                []ids.AssetID
	revision, pipeline uint64
	generation         uint32
	dirty              bool
}

func (f *drawingFrames) current() bool {
	if f == nil || f.ed == nil || f.ed.Docs == nil || f.ed.Pipeline == nil || f.ed.Loader == nil ||
		f.ed.Docs.AssetDocument(f.doc.Asset) != f.doc || f.doc.Path != f.path || f.doc.Binding.Model != f.model ||
		!f.model.PreviewCurrent(f.revision) || f.model.Session.Info().Gesture ||
		f.model.Session.Info().Dirty != f.dirty || f.ed.Pipeline.Revision() != f.pipeline || f.ed.Loader.Generation() != f.generation {
		return false
	}
	if f.doc.Metadata != nil && (f.doc.Metadata.Dirty() || f.doc.Metadata.Conflict) {
		return false
	}
	if f.dirty && !f.ed.WorkingSourceCurrent(f.doc, f.revision) {
		return false
	}
	// Recheck the disk import key immediately before applying the reference;
	// external writers do not necessarily advance Pipeline.Revision().
	return f.ed.Pipeline.VerifyCurrent(f.doc.Asset) == nil
}

func prepareDrawingFrames(ed *context.Editor, doc *document.AssetDocument) (*drawingFrames, error) {
	if ed == nil || ed.Docs == nil || ed.Pipeline == nil || ed.Loader == nil || doc == nil || ed.Docs.AssetDocument(doc.Asset) != doc {
		return nil, errors.New(i18n.Tr("editor.particles.drawing_closed", "The selected drawing is no longer open."))
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok || model.Session == nil || model.Session.Info().Gesture {
		return nil, errors.New(i18n.Tr("editor.particles.drawing_finish", "Finish the drawing gesture before assigning frames."))
	}
	if doc.Metadata != nil && (doc.Metadata.Dirty() || doc.Metadata.Conflict) {
		return nil, errors.New(i18n.Tr("editor.particles.drawing_metadata", "Save or resolve the drawing's metadata before assigning frames."))
	}
	f := &drawingFrames{ed: ed, doc: doc, model: model, path: doc.Path, revision: model.PreviewRevision(), pipeline: ed.Pipeline.Revision(), generation: ed.Loader.Generation(), dirty: model.Session.Info().Dirty}
	if f.revision == 0 || (f.dirty && !ed.WorkingSourceCurrent(doc, f.revision)) {
		return nil, errors.New(i18n.Tr("editor.particles.drawing_preview_pending", "Wait for the current drawing preview before assigning frames."))
	}
	if err := ed.Pipeline.VerifyCurrent(doc.Asset); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.Tr("editor.particles.drawing_reimport", "Reimport the saved drawing before assigning frames"), err)
	}
	frames := model.Session.Document.Info().Frames
	if len(frames) == 0 || len(frames) > 1024 {
		return nil, errors.New(i18n.Tr("editor.particles.drawing_frame_limit", "A particle flipbook requires 1–1024 frames."))
	}
	index := ed.Loader.Index()
	if index == nil {
		return nil, errors.New(i18n.Tr("editor.particles.drawing_preview_pending", "Wait for the current drawing preview before assigning frames."))
	}
	for _, frame := range frames {
		name := artsource.FrameName(frame.ID, art.Albedo)
		id, ok := ed.Pipeline.Lookup(doc.Path + pipeline.SubSep + name)
		entry, saved := ed.Pipeline.Entry(id)
		loaded, available := index.Lookup(id)
		if !ok || !saved || entry.Source != doc.Path || entry.Kind != asset.ArtSpriteRegion || entry.Missing || entry.Error != "" ||
			!available || loaded.Source != doc.Path || loaded.Kind != asset.ArtSpriteRegion || loaded.Missing || loaded.Error != "" {
			return nil, errors.New(i18n.Tr("editor.particles.drawing_save_frames", "Save the drawing and wait for import before assigning new frames."))
		}
		f.ids = append(f.ids, id)
	}
	if !f.current() {
		return nil, errors.New(i18n.Tr("editor.particles.drawing_changed", "The drawing or its preview changed; try again."))
	}
	return f, nil
}

type drawingPicker struct{ selected ids.AssetID }

func (p *drawingPicker) draw(ed *context.Editor, apply func(*document.AssetDocument) error) {
	if ed == nil || ed.UI == nil || ed.Docs == nil {
		return
	}
	var drawings []*document.AssetDocument
	for _, d := range ed.Docs.AssetDocuments() {
		if _, ok := d.Binding.Model.(*artedit.Document); ok {
			drawings = append(drawings, d)
		}
	}
	slices.SortFunc(drawings, func(a, b *document.AssetDocument) int { return strings.Compare(a.Path, b.Path) })
	if len(drawings) == 0 {
		widgets.LabelDim(ed.UI, i18n.Tr("editor.particles.drawing_open", "Open a drawing to use its frames."))
		return
	}
	names := []string{i18n.Tr("editor.particles.drawing_choose", "Choose an open drawing")}
	selected := 0
	for i, d := range drawings {
		names = append(names, d.Path)
		if d.Asset == p.selected {
			selected = i + 1
		}
	}
	ui := ed.UI
	if widgets.Combo(ui, ui.ID("particle.drawing"), widgets.Field(ui, i18n.Tr("editor.particles.drawing", "Drawing")), &selected, names, false) {
		p.selected = ids.AssetID{}
		if selected > 0 {
			p.selected = drawings[selected-1].Asset
		}
	}
	if selected == 0 {
		return
	}
	widgets.LabelWrapped(ui, i18n.Tr("editor.particles.drawing_fps_hint", "Particles use constant FPS, not individual frame durations. Save new frames before assigning them. For multiple frames, zero FPS becomes 12."), false)
	if widgets.Button(ui, i18n.Tr("editor.particles.drawing_assign", "Use drawing frames")) {
		if err := apply(ed.Docs.AssetDocument(p.selected)); err != nil {
			ed.Notify(context.LevelError, err.Error())
		}
	}
}

func askCreatePreset(ed *context.Editor, dir string) error {
	if ed == nil || ed.Pipeline == nil || ed.Docs == nil || ed.CreateAsset == nil {
		return errors.New("particle preset creation requires project asset services")
	}
	ed.Ask(context.Question{
		Title:   i18n.Tr("editor.particles.create_preset_file", "Particle Preset (.aepreset)…"),
		Fields:  []string{i18n.Tr("editor.art.source_path", "Source path"), i18n.Tr("editor.particles.preset_template", "Template: Dust / Spark / Smoke / Rain / Explosion")},
		Values:  []string{assetops.UniqueFile(ed, dir, "effect", ".aepreset"), "Dust"},
		Buttons: []string{i18n.Tr("editor.art.create", "Create"), i18n.Tr("editor.art.cancel", "Cancel")},
		Submit: func(values []string) {
			if len(values) != 2 {
				return
			}
			rel := strings.TrimSpace(values[0])
			if !strings.EqualFold(path.Ext(rel), ".aepreset") {
				ed.Notify(context.LevelError, i18n.Tr("editor.particles.preset_extension", "Preset path must end with .aepreset."))
				return
			}
			data, err := particles.PresetSource(strings.TrimSpace(values[1]))
			if err == nil {
				var id ids.AssetID
				id, err = ed.CreateAsset(rel, data)
				if err == nil {
					_, err = ed.OpenAsset(id)
				}
			}
			if err != nil {
				ed.Notify(context.LevelError, err.Error())
			}
		},
	})
	return nil
}

type presetPanel struct {
	scroll, height float32
	bar            widgets.ScrollState
	a              *adapter
	d              *document.AssetDocument
	v              transport
	surf           surface
	grad           GradientEditor
	drawing        drawingPicker
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

func (x *presetPanel) assignDrawing(doc *document.AssetDocument) error {
	ed := x.a.ed
	if x.closed || ed == nil || ed.Docs == nil || ed.Docs.AssetDocument(x.d.Asset) != x.d || x.d.Source == nil || x.d.Source.ReadOnly || x.d.Source.Conflict {
		return errors.New(i18n.Tr("editor.particles.preset_unavailable", "The particle preset is closed, read-only or in conflict."))
	}
	source, code := x.d.Source, x.d.Source.Code
	revision := code.Rev()
	frames, err := prepareDrawingFrames(ed, doc)
	if err != nil {
		return err
	}
	n, c, err := particleNode(code.Bytes(), x.d.Path)
	if err != nil {
		return err
	}
	previous, err := particles.ReadPreset(ed.Types, resolver(ed), code.Bytes())
	if err != nil {
		return err
	}
	values := make([]aetext.Value, len(frames.ids))
	for i, id := range frames.ids {
		values[i] = aetext.AssetRef(id.String())
	}
	c.SetField("frames", aetext.List(values...))
	if len(values) > 1 && previous.FrameFPS == 0 {
		c.SetField("frameFPS", aetext.Float32(12))
	}
	data := aetext.Write(n)
	candidate, err := particles.ReadPreset(ed.Types, resolver(ed), data)
	if err == nil {
		err = particles.CheckResources(&candidate, resolver(ed))
	}
	if err != nil {
		return err
	}
	if x.closed || ed.Docs.AssetDocument(x.d.Asset) != x.d || x.d.Source != source || source.Code != code || code.Rev() != revision || source.ReadOnly || source.Conflict || !frames.current() {
		return errors.New(i18n.Tr("editor.particles.drawing_changed", "The drawing or its preview changed; try again."))
	}
	code.ReplaceSnapshot(data, false)
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
	x.drawing.draw(ed, x.assignDrawing)
	x.v.buttons(ed)
	for _, field := range []struct {
		name string
		v    float32
	}{{"rate", x.v.cfg.Rate}, {"duration", x.v.cfg.Duration}, {"frameFPS", x.v.cfg.FrameFPS}, {"radius", x.v.cfg.Radius}, {"direction", x.v.cfg.Direction}, {"spread", x.v.cfg.Spread}, {"drag", x.v.cfg.Drag}} {
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
