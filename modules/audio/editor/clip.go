package editor

import (
	"slices"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/modules/audio"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	peakCols  = 256
	waveH     = 72
	handleW   = 10
	undoDepth = 64
	maxGain   = 4.0
)

var streamModes = [3][2]string{{"auto", "Auto"}, {"on", "On"}, {"off", "Off"}}

type clipEdit struct {
	id     ids.AssetID
	drag   int
	orig   int64
	acc    float32
	start  int64
	end    int64
	gain   float64
	gainOn bool
	loop   bool
	undo   []pipeline.Settings
	redo   []pipeline.Settings
}

type peakSource interface {
	Peaks(ids.AssetID, int) ([]float32, bool)
}

func formatName(f uint8) string {
	if f == asset.AudioOgg {
		return i18n.Tr("i18n.audio.ogg_vorbis", "Ogg Vorbis")
	}
	return "WAV"
}

func channels(n int) string {
	if n == 1 {
		return "mono"
	}
	return "stereo"
}

func streamMode(s pipeline.Settings) string {
	if v, ok := s["stream"]; ok && v.Kind == aetext.VBool {
		if v.Bool {
			return "on"
		}
		return "off"
	}
	return s.String("stream", "auto")
}

type ClipView struct {
	ID        ids.AssetID
	Path      string
	Format    uint8
	Rate      int
	Channels  int
	Frames    int64
	Seconds   float32
	LoopStart int64
	LoopEnd   int64
	Peaks     []float32
	PeaksDone bool
	Settings  pipeline.Settings
	Position  float32
	Playing   bool
}

func DrawClip(ed *context.Editor, v ClipView) {
	p := &plugin{ed: ed, clip: clipEdit{id: v.ID, drag: -1}}
	p.drawClip(ed, nil, v)
}

func loopOf(v ClipView) (int64, int64) {
	ls, le := v.LoopStart, v.LoopEnd
	if x := v.Settings.Int("loopStart", -1); x >= 0 {
		ls = int64(x)
	}
	if x := v.Settings.Int("loopEnd", -1); x >= 0 {
		le = int64(x)
	}
	if le <= ls || le > v.Frames {
		return 0, v.Frames
	}
	return ls, le
}

func (p *plugin) clipInspector(ed *context.Editor, id ids.AssetID) {
	ui := ed.UI
	pl := ed.Pipeline
	var mod *audio.Module
	if ed.Engine != nil {
		mod = audio.ModuleOf(ed.Engine)
	}
	if mod == nil || pl == nil || ed.Loader == nil {
		widgets.LabelDim(ui, i18n.Tr("i18n.audio.the_audio_output_is_not", "The audio output is not open"))
		return
	}
	if p.clip.id != id {
		p.clip = clipEdit{id: id, drag: -1}
	}
	meta, ok := pl.MetaOf(id)
	if !ok {
		widgets.LabelDim(ui, i18n.Tr("i18n.audio.the_sound_is_not_imported", "The sound is not imported yet"))
		return
	}
	c, err := mod.Clip(ed.Loader, id)
	if err != nil {
		widgets.LabelWrapped(ui, err.Error(), true)
		return
	}
	a := c.Art
	v := ClipView{
		ID: id, Format: a.Format, Rate: a.Rate, Channels: a.Channels, Frames: a.Frames, Seconds: c.Seconds,
		LoopStart: a.LoopStart, LoopEnd: a.LoopEnd, Settings: meta.Settings,
	}
	v.Path, _ = pl.PathOf(id)
	v.Peaks, v.PeaksDone = p.peaks(ed, id)
	if mod.PreviewClip() == id {
		v.Position, v.Playing = mod.PreviewPosition()
	}
	p.drawClip(ed, mod, v)
}

func (p *plugin) drawClip(ed *context.Editor, mod *audio.Module, v ClipView) {
	ui := ed.UI
	ui.PushID("clip/" + v.ID.String())
	defer ui.PopID()
	if v.Path != "" {
		widgets.LabelDim(ui, v.Path)
	}
	widgets.Labelf(ui, "%s, %d Hz, %s, %s s", formatName(v.Format), v.Rate, channels(v.Channels), seconds(v.Seconds))
	p.wave(ed, v)
	p.transport(ed, mod, v)
	p.importFields(ed, v.Settings)
	p.history(ed, v.Settings)
}

func (p *plugin) peaks(ed *context.Editor, id ids.AssetID) ([]float32, bool) {
	if src, ok := ed.Thumbs.(peakSource); ok {
		return src.Peaks(id, peakCols)
	}
	return nil, true
}

func (p *plugin) wave(ed *context.Editor, v ClipView) {
	ui := ed.UI
	th := &ui.Theme
	r := ui.Cursor().Next(waveH)
	ui.Box(r, th.Radius.Button, th.Background, th.Border)
	k := r.W / float32(max(v.Frames, 1))
	hid := [2]core.ID{core.IDSeq(ui.ID("loop"), 0), core.IDSeq(ui.ID("loop"), 1)}
	if p.clip.drag >= 0 && ui.Active() != hid[p.clip.drag] {
		p.commitLoop(ed, v)
	}
	ls, le := loopOf(v)
	if p.clip.drag >= 0 {
		ls, le = p.clip.start, p.clip.end
	}
	band := th.Accent
	band.A = 0.16
	xs := [2]float32{r.X + float32(ls)*k, r.X + float32(le)*k}
	if v.Frames > 0 {
		ui.Rect(mathx.Rect{X: xs[0], Y: r.Y, W: xs[1] - xs[0], H: r.H}, band)
	}
	mid := r.Y + r.H/2
	if len(v.Peaks) > 0 {
		w := r.W / float32(len(v.Peaks))
		for i, x := range v.Peaks {
			h := max(min(x, 1)*(r.H/2-3), 0.5)
			ui.Rect(mathx.Rect{X: r.X + float32(i)*w, Y: mid - h, W: max(w, 1), H: 2 * h}, th.AccentDim)
		}
	} else {
		label := i18n.Tr("i18n.audio.reading_the_waveform", "Reading the waveform")
		if v.PeaksDone {
			label = i18n.Tr("i18n.audio.no_waveform", "No waveform")
		}
		ui.TextIn(r, label, th.TextDim, core.AlignCenter)
	}
	if v.Frames <= 0 || r.W <= 0 {
		return
	}
	for i, hx := range xs {
		hr := mathx.Rect{X: hx - handleW/2, Y: r.Y, W: handleW, H: r.H}
		if d, ok := ui.Drag(hid[i], hr); ok {
			if p.clip.drag != i {
				p.clip.drag, p.clip.start, p.clip.end, p.clip.acc = i, ls, le, 0
				p.clip.orig = ls
				if i == 1 {
					p.clip.orig = le
				}
			}
			p.clip.acc += d.X
			x := p.clip.orig + int64(p.clip.acc/k)
			if i == 0 {
				p.clip.start = min(max(x, 0), p.clip.end-1)
			} else {
				p.clip.end = min(max(x, p.clip.start+1), v.Frames)
			}
		}
		col := th.AccentDim
		if ui.Active() == hid[i] || ui.Hot() == hid[i] {
			col = th.Accent
		}
		ui.Rect(mathx.Rect{X: hx - 1, Y: r.Y, W: 2, H: r.H}, col)
		ui.Rect(mathx.Rect{X: hx - handleW/2, Y: r.Y, W: handleW, H: 6}, col)
	}
	if v.Playing && v.Seconds > 0 {
		px := r.X + min(max(v.Position/v.Seconds, 0), 1)*r.W
		ui.Rect(mathx.Rect{X: px, Y: r.Y, W: 1, H: r.H}, th.Text)
	}
}

func (p *plugin) commitLoop(ed *context.Editor, v ClipView) {
	start, end := p.clip.start, p.clip.end
	p.clip.drag = -1
	if ls, le := loopOf(v); ls == start && le == end {
		return
	}
	n := v.Settings.Clone()
	n["loopStart"], n["loopEnd"] = aetext.Int(start), aetext.Int(end)
	p.write(ed, v.Settings, n)
}

func (p *plugin) write(ed *context.Editor, old, n pipeline.Settings) {
	if err := ed.Pipeline.SetSettings(p.clip.id, n); err != nil {
		p.notify(err)
		return
	}
	p.clip.undo = append(p.clip.undo, old.Clone())
	if len(p.clip.undo) > undoDepth {
		p.clip.undo = p.clip.undo[1:]
	}
	p.clip.redo = p.clip.redo[:0]
}

func (p *plugin) transport(ed *context.Editor, mod *audio.Module, v ClipView) {
	ui := ed.UI
	row := ui.Cursor().Next(0)
	w := (row.W - 8) / 3
	at := func(i int) mathx.Rect {
		return mathx.Rect{X: row.X + float32(i)*(w+4), Y: row.Y, W: w, H: row.H}
	}
	if widgets.ButtonRect(ui, ui.ID("play"), at(0), i18n.Tr("i18n.audio.play", "Play")) && mod != nil {
		p.aud = audition{}
		p.notify(mod.Preview(ed.Loader, v.ID, p.clip.loop))
	}
	if widgets.ButtonRect(ui, ui.ID("stop"), at(1), i18n.Tr("i18n.audio.stop", "Stop")) && mod != nil {
		mod.StopPreview()
	}
	widgets.ToggleButton(ui, ui.ID("looping"), at(2), i18n.Tr("i18n.audio.loop", "Loop"), &p.clip.loop)
	widgets.LabelDim(ui, i18n.Tr("i18n.audio.position_s_of_seconds_s", "{position} s of {seconds} s", i18n.Str("position", seconds(v.Position)), i18n.Str("seconds", seconds(v.Seconds))))
}

func (p *plugin) importFields(ed *context.Editor, s pipeline.Settings) {
	ui := ed.UI
	widgets.LabelDim(ui, i18n.Tr("i18n.audio.import", "Import"))
	cur := streamMode(s)
	r := widgets.Field(ui, i18n.Tr("i18n.audio.stream", "Stream"))
	w := (r.W - 8) / 3
	for i, m := range streamModes {
		on := cur == m[0]
		br := mathx.Rect{X: r.X + float32(i)*(w+4), Y: r.Y, W: w, H: r.H}
		if widgets.ToggleButton(ui, core.IDSeq(ui.ID("stream"), i), br, m[1], &on) && cur != m[0] {
			n := s.Clone()
			n["stream"] = aetext.Ident(m[0])
			p.write(ed, s, n)
		}
	}
	g := s.Float("gain", 1)
	if p.clip.gainOn {
		g = p.clip.gain
	}
	e := widgets.DragNumber(ui, ui.ID("gain"), widgets.Field(ui, i18n.Tr("i18n.audio.gain", "Gain")), &g, widgets.NumberOpts{Speed: 0.01, Prec: 2})
	if e.Changed {
		p.clip.gain, p.clip.gainOn = min(max(g, 0), maxGain), true
	}
	if e.End && p.clip.gainOn {
		p.clip.gainOn = false
		if p.clip.gain != s.Float("gain", 1) {
			n := s.Clone()
			n["gain"] = aetext.Float64(p.clip.gain)
			p.write(ed, s, n)
		}
	}
	names := audio.Buses[1:]
	cur = s.String("bus", "sfx")
	sel := slices.Index(names, cur)
	if widgets.Combo(ui, ui.ID("bus"), widgets.Field(ui, i18n.Tr("i18n.audio.bus", "Bus")), &sel, names, false) && sel >= 0 && names[sel] != cur {
		n := s.Clone()
		n["bus"] = aetext.Ident(names[sel])
		p.write(ed, s, n)
	}
}

func (p *plugin) history(ed *context.Editor, s pipeline.Settings) {
	ui := ed.UI
	undo, redo := twoButtons(ui)
	if widgets.ButtonRect(ui, ui.ID("undo"), undo, i18n.Tr("i18n.audio.undo", "Undo")) && len(p.clip.undo) > 0 {
		prev := p.clip.undo[len(p.clip.undo)-1]
		if err := ed.Pipeline.SetSettings(p.clip.id, prev); err != nil {
			p.notify(err)
			return
		}
		p.clip.undo = p.clip.undo[:len(p.clip.undo)-1]
		p.clip.redo = append(p.clip.redo, s.Clone())
	}
	if widgets.ButtonRect(ui, ui.ID("redo"), redo, i18n.Tr("i18n.audio.redo", "Redo")) && len(p.clip.redo) > 0 {
		next := p.clip.redo[len(p.clip.redo)-1]
		if err := ed.Pipeline.SetSettings(p.clip.id, next); err != nil {
			p.notify(err)
			return
		}
		p.clip.redo = p.clip.redo[:len(p.clip.redo)-1]
		p.clip.undo = append(p.clip.undo, s.Clone())
	}
}
