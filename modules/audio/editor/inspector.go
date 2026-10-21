package editor

import (
	"strconv"
	"time"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/modules/audio"
	"aego/scene"
	"aego/ui/core"
	"aego/ui/widgets"
)

func twoButtons(ui *core.Context) (mathx.Rect, mathx.Rect) {
	row := ui.Cursor().Next(0)
	w := (row.W - 4) / 2
	return mathx.Rect{X: row.X, Y: row.Y, W: w, H: row.H}, mathx.Rect{X: row.X + w + 4, Y: row.Y, W: w, H: row.H}
}

func seconds(v float32) string {
	return strconv.FormatFloat(float64(v), 'f', 1, 32)
}

func progress(ui *core.Context, pos, length float32, on bool) {
	th := &ui.Theme
	bar := ui.Cursor().Next(6)
	ui.RoundRect(bar, 3, th.Widget)
	if length > 0 {
		k := mathx.Clamp(pos/length, 0, 1)
		col := th.AccentDim
		if on {
			col = th.Accent
		}
		ui.RoundRect(mathx.Rect{X: bar.X, Y: bar.Y, W: bar.W * k, H: bar.H}, 3, col)
	}
	text := seconds(pos) + " s"
	if length > 0 {
		text += " of " + seconds(length) + " s"
	}
	widgets.LabelDim(ui, text)
}

func (p *plugin) sourceInspector(ctx *context.Editor, nodes []ids.NodeID, def func()) {
	def()
	d := ctx.Doc()
	if d == nil || len(nodes) != 1 {
		return
	}
	ui := ctx.UI
	h := d.Scene().Handle(nodes[0])
	if svc := audio.Of(p.world()); svc != nil {
		p.playControls(ui, svc, h)
		return
	}
	if c, ok := scene.Get[audio.AudioSource2D](d.Scene(), h); ok {
		p.previewControls(ui, d.Scene(), c)
	}
}

func (p *plugin) playControls(ui *core.Context, svc *audio.Service, h scene.NodeHandle) {
	play, stop := twoButtons(ui)
	if widgets.ButtonRect(ui, ui.ID("audio.play"), play, "Play") {
		svc.PlaySource(h)
	}
	if widgets.ButtonRect(ui, ui.ID("audio.stop"), stop, "Stop") {
		svc.StopSource(h, 0)
	}
	if c, ok := scene.Get[audio.AudioSource2D](svc.Scene(), h); ok {
		progress(ui, c.Position, svc.SourceLength(h), c.Playing)
	}
}

func (p *plugin) previewControls(ui *core.Context, s *scene.Scene, c *audio.AudioSource2D) {
	mod := p.module()
	if mod == nil {
		return
	}
	if c.Clip.IsZero() {
		if !c.Event.IsZero() {
			widgets.LabelDim(ui, "Events are heard in Play; Preview plays Clip")
		}
		return
	}
	play, stop := twoButtons(ui)
	if widgets.ButtonRect(ui, ui.ID("audio.preview"), play, "Preview") {
		p.notify(mod.Preview(s.Resolver(), c.Clip.ID, c.Loop))
	}
	if widgets.ButtonRect(ui, ui.ID("audio.unpreview"), stop, "Stop") {
		mod.StopPreview()
	}
	clip, err := mod.Clip(s.Resolver(), c.Clip.ID)
	if err != nil {
		widgets.LabelWrapped(ui, err.Error(), true)
		return
	}
	pos, on := mod.PreviewPosition()
	if mod.PreviewClip() != c.Clip.ID {
		pos, on = 0, false
	}
	progress(ui, pos, clip.Seconds, on)
}

func ms(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func (p *plugin) profile(ed *context.Editor) {
	ui := ed.UI
	mod := p.module()
	if mod == nil || mod.Mixer() == nil {
		widgets.LabelDim(ui, "The audio output is not open")
		return
	}
	st, c, spec := mod.Stats(), mod.Counts(), mod.Spec()
	world := 0
	if svc := audio.Of(p.world()); svc != nil {
		world = svc.Voices()
	}
	widgets.Labelf(ui, "Output %d Hz, %d frames per block", spec.SampleRate, spec.Frames)
	widgets.Labelf(ui, "Voices %d, in this world %d", st.Voices, world)
	widgets.Labelf(ui, "Mix %.2f ms max, %.2f ms average", ms(st.MixMax), ms(st.MixAvg))
	widgets.Labelf(ui, "Underruns %d, dropped commands %d, blocks %d", st.Underruns, st.Dropped, st.Blocks)
	widgets.Labelf(ui, "Finished %d, stolen %d, rejected %d, starved %d", c.Finished, c.Stolen, c.Rejected, c.Starved)
}
