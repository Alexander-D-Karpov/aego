package editor

import (
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/preview"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/engine/i18n"
	"aego/modules/particles"
	"aego/scene"
	"aego/scene/frame"
	"aego/ui/core"
	"aego/ui/widgets"
	"fmt"
)

type transport struct {
	sim            *particles.Preview
	cfg            particles.ParticleSystem2D
	speed          float32
	seed           uint64
	locked, paused bool
	showSettings   bool
	bad            string
}

func resolver(ed *context.Editor) asset.Resolver {
	if ed.Loader == nil {
		return nil
	}
	return ed.Loader
}
func (v *transport) init(ed *context.Editor) {
	if v.sim == nil {
		v.sim = particles.NewPreview(ed.Types, resolver(ed))
		v.speed = 1
		v.seed = 42
		v.locked = true
	}
}
func (v *transport) configure(ed *context.Editor, c particles.ParticleSystem2D, first bool) {
	v.init(ed)
	if err := v.sim.Configure(c, v.seed); err != nil {
		v.bad = err.Error()
		return
	}
	first = first || v.cfg.MaxParticles == 0
	v.bad = ""
	v.cfg = c
	if first && !c.PlayOnStart {
		v.restart()
	}
}
func (v *transport) restart() {
	if v.sim == nil {
		return
	}
	v.sim.Restart()
	if !v.cfg.PlayOnStart && v.cfg.Rate == 0 && len(v.cfg.Bursts) == 0 {
		v.sim.Burst(8)
	}
	v.paused = false
}
func (v *transport) close() {
	if v.sim != nil {
		v.sim.Close()
		v.sim = nil
	}
}
func (v *transport) buttons(ed *context.Editor) {
	ui := ed.UI
	if ui == nil || v.sim == nil {
		return
	}
	if v.bad != "" {
		widgets.LabelWrapped(ui, v.bad, true)
	}
	play := v.paused || !v.sim.Playing()
	toggle, toggleTip := i18n.Tr("editor.particles.pause_short", "Pause"), i18n.Tr("editor.particles.pause", "Pause preview")
	if play {
		toggle, toggleTip = i18n.Tr("editor.common.play", "Play"), i18n.Tr("editor.particles.play", "Play preview")
	}
	labels := [4]string{toggle, i18n.Tr("editor.particles.restart_short", "Restart"), i18n.Tr("editor.particles.burst", "Burst (8)"), i18n.Tr("editor.particles.clear_short", "Clear")}
	tips := [4]string{toggleTip, i18n.Tr("editor.particles.restart", "Restart preview"), labels[2], i18n.Tr("editor.particles.clear", "Clear particles")}
	var need float32
	for _, label := range labels {
		need = max(need, ui.TextSize(label).X+2*ui.Theme.Padding+4)
	}
	const gap = float32(4)
	cols := transportColumns(ui.Cursor().W, need, gap)
	var row mathx.Rect
	for i, label := range labels {
		if i%cols == 0 {
			row = ui.Cursor().Next(0)
		}
		w := (row.W - gap*float32(cols-1)) / float32(cols)
		rect := mathx.Rect{X: row.X + float32(i%cols)*(w+gap), Y: row.Y, W: w, H: row.H}
		id := core.IDSeq(ui.ID("particle.transport"), i+1)
		hit := widgets.ButtonRect(ui, id, rect, label)
		widgets.Tooltip(ui, id, tips[i])
		if !hit {
			continue
		}
		switch i {
		case 0:
			if play && !v.paused {
				v.sim.Play()
			}
			v.paused = !play
		case 1:
			if !v.locked {
				v.seed++
				_ = v.sim.Configure(v.cfg, v.seed)
			}
			v.restart()
		case 2:
			v.sim.Burst(8)
		case 3:
			v.sim.Clear()
		}
	}
	widgets.LabelDim(ui, fmt.Sprintf("%s: %d", i18n.Tr("editor.particles.alive", "Alive"), v.sim.Count()))
	if widgets.TreeNode(ui, ui.ID("fx.preview.options"), i18n.Tr("editor.particles.preview_options", "Preview settings"), &v.showSettings, false) {
		v.showSettings = !v.showSettings
	}
	if v.showSettings {
		widgets.Checkbox(ui, i18n.Tr("editor.particles.lock_seed", "Lock preview seed"), &v.locked)
		widgets.DragFloat(ui, ui.ID("fx.speed"), i18n.Tr("editor.particles.speed", "Preview speed"), &v.speed, .02)
		v.speed = mathx.Clamp(v.speed, 0, 4)
	}
}

func transportColumns(width, buttonWidth, gap float32) int {
	if width >= 4*buttonWidth+3*gap {
		return 4
	}
	if width >= 2*buttonWidth+gap {
		return 2
	}
	return 1
}

type adapter struct {
	panels map[document.ID]*presetPanel
	ed     *context.Editor
	reg    *ext.Registry
	source *scene.Scene
	id     ids.NodeID
	rev    uint64
	gen    uint32
	last   float64
	sel    transport
}

func Hook(r *ext.Registry) error {
	ed := r.Editor()
	if ed == nil {
		return nil
	}
	p := &adapter{ed: ed, reg: r, panels: map[document.ID]*presetPanel{}}
	if err := registerInspector(r, p); err != nil {
		return err
	}
	if ed.Docs == nil {
		return nil
	}
	unsub := ed.Docs.Subscribe(func(evt document.Event) {
		if evt.Kind == document.EvClosed {
			if x := p.panels[evt.Doc]; x != nil && ed.Docs.AssetDocument(x.d.Asset) == nil {
				x.dispose()
				p.reg.ClosePanel(x.ID())
			}
		}
	})
	if ed.Engine != nil {
		ed.Engine.OnClose(func() {
			unsub()
			p.sel.close()
			for _, x := range p.panels {
				x.dispose()
			}
		})
	}
	ed.ScenePreviews = append(ed.ScenePreviews, func(runner *frame.Runner) {
		if _, off := r.Disabled()[particles.ID]; off {
			return
		}
		if w := ed.Docs.ActiveWorld(); w != nil {
			if live := particles.Of(w); live != nil {
				live.AppendToView(runner)
			}
			return
		}
		if runner.Scene() != p.source || p.source == nil || p.sel.sim == nil {
			return
		}
		h := p.source.Handle(p.id)
		if !p.source.IsAlive(h) || runner.Hidden(h) {
			return
		}
		pose := p.source.RenderWorld(h)
		if p.source.EffectiveEnabled(h) && !p.source.Under3D(h) {
			p.sel.sim.ExtractLayers(runner, pose, p.source.Layers(h))
		}
	})
	r.Tick(p.tick)
	r.Profiler(i18n.T("editor.particles.profiler", "Particles"), true, func(ed *context.Editor) {
		if ed.Play == nil || ed.Play.World() == nil {
			return
		}
		s := particles.Of(ed.Play.World())
		if s == nil {
			return
		}
		st := s.Stats()
		widgets.LabelDim(ed.UI, fmt.Sprintf("%s: %d   %d / %d", i18n.Tr("editor.particles.emitters", "Emitters"), st.Emitters, st.Alive, st.Capacity))
		widgets.LabelDim(ed.UI, fmt.Sprintf("%s: %d / %d", i18n.Tr("editor.particles.spawned_dropped", "Born / dropped this frame"), st.BornThisFrame, st.DroppedThisFrame))
		widgets.LabelDim(ed.UI, fmt.Sprintf("CPU ms: %.3f / %.3f / %.3f", float64(st.SimulateNS)/1e6, float64(st.ExtractNS)/1e6, float64(st.SortNS)/1e6))
		widgets.LabelDim(ed.UI, fmt.Sprintf("%s: %d / %d", i18n.Tr("editor.particles.culling", "Visible / culled emitters"), st.VisibleEmitters, st.CulledEmitters))
		widgets.LabelDim(ed.UI, fmt.Sprintf("%s: %d / %d B / %d", i18n.Tr("editor.particles.uploads", "Uploads / bytes / draws"), st.InstanceUploads, st.UploadBytes, st.Draws))
	})
	open := r.DocumentPanelFactory(func(d *document.AssetDocument) workspace.Panel { return p.panel(d) })
	if err := r.DocumentHandler(document.Handler{ID: "particles.preset", Name: i18n.T("editor.particles.preset", "Particle preset"), Icon: "file.preset", Kinds: []asset.ArtifactKind{asset.ArtPreset}, Roles: []string{particles.NameSystem}, TextSource: true, Capabilities: document.View | document.EditSource | document.Preview | document.RevealSource, Open: open}); err != nil {
		return err
	}
	if err := r.CreateAction(context.CreateAction{
		Key: "particles.create-preset", Name: i18n.Tr("editor.particles.create_preset_file", "Particle Preset (.aepreset)…"),
		Category: i18n.Tr("editor.assets.create_category", "Files"), Icon: "file.preset",
		Run:         func(ids.NodeID) error { return askCreatePreset(ed, ed.AssetCreateDirectory("assets")) },
		RunInFolder: func(dir string) error { return askCreatePreset(ed, dir) },
	}); err != nil {
		return err
	}
	return r.Thumbnail(asset.ArtPreset, preview.Handler{Component: particles.NameSystem, ReadSource: true, Version: "particles.v2", Render: p.thumbnail, Build: p.card})
}
func (p *adapter) tick() {
	ed := p.ed
	sc := ed.Scene()
	var id ids.NodeID
	if sc != nil && !ed.Doc().IsPlay() {
		if sel := ed.Selected(); len(sel) == 1 {
			id = sel[0]
		}
	}
	var c *particles.ParticleSystem2D
	if sc != nil && !id.IsZero() {
		h := sc.Handle(id)
		if sc.IsAlive(h) {
			sc.RenderWorld(h)
			if sc.EffectiveEnabled(h) && !sc.Under3D(h) {
				c, _ = scene.Get[particles.ParticleSystem2D](sc, h)
			}
		}
	}
	if c == nil {
		p.sel.close()
		p.source = nil
		p.last = 0
		return
	}
	now := ed.Now()
	first := p.source != sc || p.id != id
	if first {
		p.sel.close()
		p.sel = transport{}
		p.source = sc
		p.id = id
		p.last = now
	}
	var gen uint32
	if ed.Loader != nil {
		gen = ed.Loader.Generation()
	}
	if first || sc.Revision() != p.rev || gen != p.gen {
		p.sel.init(ed)
		p.sel.sim.Step(0, sc.RenderWorld(sc.Handle(id)))
		p.sel.configure(ed, *c, first)
		p.rev = sc.Revision()
		p.gen = gen
	}
	if !p.sel.paused && p.sel.sim != nil {
		p.sel.sim.Step(max(0, min(now-p.last, .1))*float64(p.sel.speed), sc.RenderWorld(sc.Handle(id)))
	}
	p.last = now
}
func (p *adapter) controls(ed *context.Editor, id ids.NodeID) {
	if p.id == id && p.sel.sim != nil {
		p.sel.buttons(ed)
	}
}
