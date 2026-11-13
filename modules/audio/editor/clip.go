package editor

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/modules/audio"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	peakCols = 256
	waveH    = 72
	handleW  = 10
	maxGain  = 4.0
)

var streamModes = [3][2]string{{"auto", "Auto"}, {"on", "On"}, {"off", "Off"}}

func streamLabels() [3]string {
	return [3]string{i18n.Tr("i18n.audio.stream_auto", "Auto"), i18n.Tr("i18n.audio.stream_on", "On"), i18n.Tr("i18n.audio.stream_off", "Off")}
}

type clipEdit struct {
	id           ids.AssetID
	drag         int
	orig         int64
	acc          float32
	start        int64
	end          int64
	loopSettings pipeline.Settings
	gain         float64
	gainOn       bool
	gainSettings pipeline.Settings
	loop         bool
	waveStart    int64
	waveEnd      int64
	navGrab      int64
	navigating   bool
	selectAnchor int64
	selectEnd    int64
	selecting    bool
	docScroll    float32
	docContent   float32
	docScrollBar widgets.ScrollState
}

type peakSource interface {
	Peaks(ids.AssetID, int) ([]float32, bool)
}

type stereoPeakSource interface {
	StereoPeaks(ids.AssetID, int) ([]float32, bool)
}

type stereoWindowPeakSource interface {
	StereoPeaksWindow(ids.AssetID, int64, int64, int) ([]float32, bool)
}

type monoWindowPeakSource interface {
	MonoPeaksWindow(ids.AssetID, int64, int64, int) ([]float32, bool)
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
	source    *audio.Clip
	Path      string
	Format    uint8
	Rate      int
	Channels  int
	Frames    int64
	Seconds   float32
	LoopStart int64
	LoopEnd   int64
	Peaks     []float32
	// StereoPeaks interleaves left and right amplitudes for each column.
	StereoPeaks []float32
	WaveStart   int64
	WaveEnd     int64
	WaveNav     bool
	// WaveSelect allows view-only selection without promising zoom support.
	WaveSelect bool
	PeaksDone  bool
	Settings   pipeline.Settings
	Position   float32
	Playing    bool
	Paused     bool
	Preparing  bool
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
	p.clipInspectorView(ed, id, false, mathx.Rect{})
}

func (p *plugin) clipInspectorView(ed *context.Editor, id ids.AssetID, documentView bool, documentArea mathx.Rect) {
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
	if err := mod.TakePreviewStreamSeekError(); err != nil {
		p.notify(localizeAudioSeekError(err))
	}
	if p.clip.id != id {
		p.clip = clipEdit{id: id, drag: -1}
	}
	doc, err := ed.DocumentForAsset(id)
	if err != nil || doc.Metadata == nil {
		widgets.LabelDim(ui, i18n.Tr("i18n.audio.the_sound_is_not_imported", "The sound is not imported yet"))
		return
	}
	meta, err := asset.ParseSourceMeta(doc.Metadata.Code.Bytes(), doc.Metadata.Path)
	if err != nil {
		widgets.LabelWrapped(ui, err.Error(), true)
		return
	}
	c, err := mod.Clip(ed.Loader, id)
	if err != nil {
		widgets.LabelWrapped(ui, err.Error(), true)
		return
	}
	a := c.Art
	v := ClipView{
		ID: id, source: c, Format: a.Format, Rate: a.Rate, Channels: a.Channels, Frames: a.Frames, Seconds: c.Seconds,
		LoopStart: a.LoopStart, LoopEnd: a.LoopEnd, Settings: meta.Settings,
	}
	v.Path, _ = pl.PathOf(id)
	p.clipPeaks(ed, &v, documentView)
	if mod.PreviewClip() == id {
		v.Position, v.Playing = mod.PreviewPosition()
		v.Paused = v.Playing && mod.PreviewPaused()
	}
	if documentView {
		p.drawClipDocument(ed, mod, v, documentArea)
	} else {
		p.drawClip(ed, mod, v)
	}
}

func (p *plugin) clipPeaks(ed *context.Editor, v *ClipView, documentView bool) {
	v.WaveSelect = false
	if documentView && v.Channels == 1 {
		if src, ok := ed.Thumbs.(monoWindowPeakSource); ok {
			start, end := audioWaveBounds(p.clip.waveStart, p.clip.waveEnd, v.Frames)
			v.Peaks, v.PeaksDone = src.MonoPeaksWindow(v.ID, start, end, peakCols)
			if len(v.Peaks) > 0 {
				v.WaveStart, v.WaveEnd, v.WaveNav = start, end, true
			}
			if v.PeaksDone && len(v.Peaks) == 0 {
				v.Peaks, v.PeaksDone = p.peaks(ed, v.ID)
			}
		} else {
			v.Peaks, v.PeaksDone = p.peaks(ed, v.ID)
		}
	} else if documentView && v.Channels == 2 {
		if src, ok := ed.Thumbs.(stereoWindowPeakSource); ok {
			start, end := audioWaveBounds(p.clip.waveStart, p.clip.waveEnd, v.Frames)
			v.StereoPeaks, v.PeaksDone = src.StereoPeaksWindow(v.ID, start, end, peakCols)
			if len(v.StereoPeaks) > 0 {
				v.WaveStart, v.WaveEnd, v.WaveNav = start, end, true
			}
			if v.PeaksDone && len(v.StereoPeaks) == 0 {
				v.Peaks, v.PeaksDone = p.peaks(ed, v.ID)
			}
		} else if src, ok := ed.Thumbs.(stereoPeakSource); ok {
			v.StereoPeaks, v.PeaksDone = src.StereoPeaks(v.ID, peakCols)
			if v.PeaksDone && len(v.StereoPeaks) == 0 {
				v.Peaks, v.PeaksDone = p.peaks(ed, v.ID)
			}
		} else {
			v.Peaks, v.PeaksDone = p.peaks(ed, v.ID)
		}
	} else {
		v.Peaks, v.PeaksDone = p.peaks(ed, v.ID)
	}
	// The mono/fallback waveform has no viewport pyramid, but can still select
	// source frames across its full displayed range without modifying the clip.
	if documentView && !v.WaveNav && len(v.Peaks) > 0 && v.Frames > 0 {
		v.WaveSelect = true
	}
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

// The centre document already provides the source path and the shared
// Save/Undo/Redo toolbar. Keep the useful waveform and controls without
// rendering another copy of those actions across the full window width.
func (p *plugin) drawClipDocument(ed *context.Editor, mod *audio.Module, v ClipView, area mathx.Rect) {
	ui := ed.UI
	ui.PushID("clip/" + v.ID.String())
	defer ui.PopID()
	cur := ui.Cursor()
	top, fullWidth := cur.Y, cur.W
	viewport := mathx.Rect{X: cur.X, Y: top, W: max(fullWidth, 0), H: max(0, area.Y+area.H-top)}
	if !v.WaveNav {
		p.clip.navigating = false
	}
	if p.clip.drag < 0 && !p.clip.selecting && !p.clip.navigating && ui.MouseOver(ui.ID("document-scroll"), viewport) {
		p.clip.docScroll -= ui.ScrollY(ui.Theme.RowHeight * 3)
	}
	p.clip.docScroll = mathx.Clamp(p.clip.docScroll, 0, max(0, p.clip.docContent-viewport.H))
	cur.Y -= p.clip.docScroll
	if p.clip.docContent > viewport.H {
		cur.W = max(0, cur.W-ui.Theme.ScrollbarW)
	}
	defer func() {
		p.clip.docContent = max(0, cur.Y+p.clip.docScroll-top)
		p.clip.docScroll = mathx.Clamp(p.clip.docScroll, 0, max(0, p.clip.docContent-viewport.H))
		cur.W = fullWidth
		track := mathx.Rect{X: viewport.X + viewport.W - ui.Theme.ScrollbarW, Y: viewport.Y, W: ui.Theme.ScrollbarW, H: viewport.H}
		widgets.ScrollBar(ui, ui.ID("document-scrollbar"), track, &p.clip.docScroll, p.clip.docContent, viewport.H, &p.clip.docScrollBar)
	}()
	width := cur.W
	waveWidth, controlsWidth := clipDocumentWidths(width)
	cur.W = waveWidth
	widgets.LabelWrapped(ui, fmt.Sprintf("%s, %d Hz, %s, %s s", formatName(v.Format), v.Rate, channels(v.Channels), seconds(v.Seconds)), false)
	if v.WaveNav {
		p.waveNavigation(ed, v)
		r := drawAudioRuler(ui, v)
		p.seekRuler(ui, mod, v, r)
	}
	p.drawWave(ed, v, clipDocumentWaveHeight(viewport.H, width, cur.RowH, cur.Space))
	if v.WaveNav || v.WaveSelect {
		p.waveSelectionLabel(ui, v)
	}
	cur.W = controlsWidth
	p.transport(ed, mod, v)
	p.importFields(ed, v.Settings)
	p.metadataConflict(ed)
}

func clipDocumentWidths(width float32) (wave, controls float32) {
	return min(max(width, 0), 900), min(max(width, 0), 440)
}

// Keep room for the transport and import fields while giving a tall document
// a larger waveform than the compact Inspector. Very small areas retain the
// compact waveform and the enclosing document scroll keeps controls reachable.
func clipDocumentWaveHeight(height, width, rowHeight, spacing float32) float32 {
	if math.IsNaN(float64(height)) || math.IsInf(float64(height), 0) || height <= 0 ||
		math.IsNaN(float64(width)) || math.IsInf(float64(width), 0) ||
		math.IsNaN(float64(rowHeight)) || math.IsInf(float64(rowHeight), 0) || rowHeight <= 0 ||
		math.IsNaN(float64(spacing)) || math.IsInf(float64(spacing), 0) || spacing < 0 {
		return waveH
	}
	rows := float32(15)
	if width < 220 {
		rows = 20 // wrapped translated labels need more vertical space
	}
	return min(max(height-rows*(rowHeight+spacing), waveH), 260)
}

func (p *plugin) peaks(ed *context.Editor, id ids.AssetID) ([]float32, bool) {
	if src, ok := ed.Thumbs.(peakSource); ok {
		return src.Peaks(id, peakCols)
	}
	return nil, true
}

type audioWaveAction uint8

const (
	waveZoomIn audioWaveAction = iota
	waveZoomOut
	wavePanLeft
	wavePanRight
)

func audioWaveBounds(start, end, total int64) (int64, int64) {
	if total <= 0 {
		return 0, 0
	}
	if start < 0 || end <= start || end > total {
		return 0, total
	}
	return start, end
}

func adjustAudioWave(start, end, total int64, action audioWaveAction) (int64, int64) {
	start, end = audioWaveBounds(start, end, total)
	if total <= 0 {
		return 0, 0
	}
	span := end - start
	switch action {
	case wavePanLeft:
		start -= min(start, max(span/2, 1))
		return start, start + span
	case wavePanRight:
		start += min(total-span-start, max(span/2, 1))
		return start, start + span
	case waveZoomIn, waveZoomOut:
		size := span
		if action == waveZoomIn {
			size = min(span, max(span/2, min(total, 512)))
		} else if span > total-span {
			size = total
		} else {
			size = span * 2
		}
		center := start + span/2
		start = max(center-size/2, 0)
		start = min(start, total-size)
		return start, start + size
	default:
		return start, end
	}
}

func (p *plugin) waveNavigation(ed *context.Editor, v ClipView) {
	ui := ed.UI
	row := ui.Cursor().Next(0)
	w := min(max((row.W-12)/4, 0), 28)
	buttons := []struct {
		label, tooltip string
		action         audioWaveAction
	}{
		{"<", i18n.Tr("i18n.audio.wave_pan_left", "Pan waveform left"), wavePanLeft},
		{"-", i18n.Tr("i18n.audio.wave_zoom_out", "Zoom waveform out"), waveZoomOut},
		{"+", i18n.Tr("i18n.audio.wave_zoom_in", "Zoom waveform in"), waveZoomIn},
		{">", i18n.Tr("i18n.audio.wave_pan_right", "Pan waveform right"), wavePanRight},
	}
	for i, button := range buttons {
		id := core.IDSeq(ui.ID("wave-nav"), i)
		r := mathx.Rect{X: row.X + float32(i)*(w+4), Y: row.Y, W: w, H: row.H}
		if widgets.ButtonRect(ui, id, r, button.label) {
			p.clip.waveStart, p.clip.waveEnd = adjustAudioWave(v.WaveStart, v.WaveEnd, v.Frames, button.action)
		}
		widgets.Tooltip(ui, id, button.tooltip)
	}
	p.waveNavigator(ui, v)
	if v.Rate > 0 {
		widgets.LabelWrapped(ui, i18n.Tr("i18n.audio.wave_visible", "Visible: {start} - {end} s",
			i18n.Str("start", seconds(float32(v.WaveStart)/float32(v.Rate))),
			i18n.Str("end", seconds(float32(v.WaveEnd)/float32(v.Rate)))), true)
	}
}

// waveNavigator is a view-only map of the full source duration. Moving the
// highlighted interval changes the displayed range, not the playback position
// or selection. Its frame mapping follows the same bounds as the waveform.
func (p *plugin) waveNavigator(ui *core.Context, v ClipView) {
	if v.Frames <= 0 || v.Frames > 1<<53 {
		p.clip.navigating = false
		return
	}
	r := ui.Cursor().Next(16)
	if r.W <= 0 || math.IsNaN(float64(r.W)) || math.IsInf(float64(r.W), 0) ||
		math.IsNaN(float64(r.X)) || math.IsInf(float64(r.X), 0) || math.IsInf(float64(r.X+r.W), 0) ||
		math.IsNaN(float64(r.Y)) || math.IsInf(float64(r.Y), 0) {
		p.clip.navigating = false
		return
	}
	start, end := audioWaveBounds(v.WaveStart, v.WaveEnd, v.Frames)
	span := end - start
	left := r.X + float32(float64(start)/float64(v.Frames)*float64(r.W))
	right := r.X + float32(float64(end)/float64(v.Frames)*float64(r.W))
	// Keep a short interval visible even when zoomed far into a long clip.
	visualWidth := min(max(right-left, 3), r.W)
	visualX := min(max(left+(right-left-visualWidth)/2, r.X), r.X+r.W-visualWidth)
	ui.Box(r, ui.Theme.Radius.Button, ui.Theme.Widget, ui.Theme.Border)
	ui.Box(mathx.Rect{X: visualX, Y: r.Y, W: visualWidth, H: r.H}, ui.Theme.Radius.Button, ui.Theme.AccentDim, ui.Theme.Accent)
	id := ui.ID("wave-navigator")
	if _, active := ui.Drag(id, r); active {
		if frame, ok := audioWaveFrame(ui.Mouse().X, r, 0, v.Frames); ok {
			if !p.clip.navigating {
				p.clip.navGrab = span / 2
				if ui.Mouse().X >= visualX && ui.Mouse().X <= visualX+visualWidth {
					p.clip.navGrab = min(max(frame-start, 0), span)
				}
			}
			next := min(max(frame-p.clip.navGrab, 0), v.Frames-span)
			p.clip.waveStart, p.clip.waveEnd = next, next+span
			p.clip.navigating = ui.Active() == id
		} else if ui.Active() != id {
			p.clip.navigating = false
		}
	} else {
		p.clip.navigating = false
	}
	widgets.Tooltip(ui, id, i18n.Tr("i18n.audio.wave_navigator", "Drag to navigate the waveform; playback position is unchanged"))
}

// audioWaveFrame maps a pointer to a source-relative frame boundary. It never
// moves the playhead or modifies source data, metadata, or document history.
func audioWaveFrame(x float32, r mathx.Rect, start, end int64) (int64, bool) {
	if start < 0 || end <= start || end > 1<<53 || r.W <= 0 ||
		math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) ||
		math.IsNaN(float64(r.X)) || math.IsInf(float64(r.X), 0) ||
		math.IsNaN(float64(r.W)) || math.IsInf(float64(r.W), 0) || math.IsInf(float64(r.X+r.W), 0) {
		return 0, false
	}
	if x <= r.X {
		return start, true
	}
	if x >= r.X+r.W {
		return end, true
	}
	fraction := float64(x-r.X) / float64(r.W)
	return min(start+int64(math.Round(fraction*float64(end-start))), end), true
}

func (c *clipEdit) waveSelection(total int64) (int64, int64, bool) {
	// View state only: an eventual edit must additionally bind the selection
	// to the imported generation and guarded source digest before using it.
	if total <= 0 {
		return 0, 0, false
	}
	start := min(max(min(c.selectAnchor, c.selectEnd), 0), total)
	end := min(max(max(c.selectAnchor, c.selectEnd), 0), total)
	return start, end, end > start
}

func audioWaveSelectionRect(r mathx.Rect, start, end, a, b int64) (mathx.Rect, bool) {
	if start < 0 || end <= start || a < 0 || b <= a || r.W <= 0 || r.H <= 0 ||
		math.IsNaN(float64(r.X)) || math.IsInf(float64(r.X), 0) ||
		math.IsNaN(float64(r.Y)) || math.IsInf(float64(r.Y), 0) ||
		math.IsNaN(float64(r.W)) || math.IsInf(float64(r.W), 0) || math.IsInf(float64(r.X+r.W), 0) ||
		math.IsNaN(float64(r.H)) || math.IsInf(float64(r.H), 0) || math.IsInf(float64(r.Y+r.H), 0) {
		return mathx.Rect{}, false
	}
	left, right := max(a, start), min(b, end)
	if right <= left {
		return mathx.Rect{}, false
	}
	scale := float64(r.W) / float64(end-start)
	x0 := min(max(r.X+float32(float64(left-start)*scale), r.X), r.X+r.W)
	x1 := min(max(r.X+float32(float64(right-start)*scale), x0), r.X+r.W)
	return mathx.Rect{X: x0, Y: r.Y, W: x1 - x0, H: r.H}, x1 > x0
}

func formatWaveSelection(v ClipView, start, end int64) string {
	time := func(frame int64) string {
		return strconv.FormatFloat(float64(frame)/float64(v.Rate), 'f', 3, 64)
	}
	return i18n.Tr("i18n.audio.wave_selection", "Selected {frames} frames: {start} - {end} s",
		i18n.Str("frames", strconv.FormatInt(end-start, 10)), i18n.Str("start", time(start)), i18n.Str("end", time(end)))
}

func (p *plugin) waveSelectionLabel(ui *core.Context, v ClipView) {
	start, end, ok := p.clip.waveSelection(v.Frames)
	if !ok || v.Rate <= 0 {
		return
	}
	widgets.LabelWrapped(ui, formatWaveSelection(v, start, end), true)
}

func (p *plugin) wave(ed *context.Editor, v ClipView) {
	p.drawWave(ed, v, waveH)
}

func (p *plugin) drawWave(ed *context.Editor, v ClipView, height float32) {
	ui := ed.UI
	th := &ui.Theme
	r := ui.Cursor().Next(height)
	ui.Box(r, th.Radius.Button, th.Background, th.Border)
	start, end := audioWaveBounds(v.WaveStart, v.WaveEnd, v.Frames)
	k := r.W / float32(max(end-start, 1))
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
	xs := [2]float32{r.X + float32(ls-start)*k, r.X + float32(le-start)*k}
	if v.Frames > 0 {
		left, right := max(ls, start), min(le, end)
		if right > left {
			ui.Rect(mathx.Rect{X: r.X + float32(left-start)*k, Y: r.Y, W: float32(right-left) * k, H: r.H}, band)
		}
	}
	selection := mathx.Rect{}
	if v.WaveNav || v.WaveSelect {
		if a, b, ok := p.clip.waveSelection(v.Frames); ok {
			if visible, ok := audioWaveSelectionRect(r, start, end, a, b); ok {
				selection = visible
				col := th.Selection
				col.A = .22
				ui.Rect(selection, col)
			}
		}
	}
	mid := r.Y + r.H/2
	if len(v.StereoPeaks) > 0 && len(v.StereoPeaks)%2 == 0 {
		cols := len(v.StereoPeaks) / 2
		for ch := range 2 {
			for i := range cols {
				if peak, ok := stereoWaveRect(r, cols, i, ch, v.StereoPeaks[i*2+ch]); ok {
					ui.Rect(peak, th.Accent)
				}
			}
			label := "L"
			if ch == 1 {
				label = "R"
			}
			if width := min(max(r.W-3, 0), 14); width > 0 {
				height := min(th.RowHeight, r.H/2)
				box := mathx.Rect{X: r.X + 3, Y: r.Y + float32(ch)*r.H/2 + (r.H/2-height)/2, W: width, H: height}
				ui.Rect(box, th.Background)
				ui.TextIn(box, label, th.Text, core.AlignLeft)
			}
		}
	} else if len(v.Peaks) > 0 {
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
	if selection.W > 0 {
		ui.Rect(mathx.Rect{X: selection.X, Y: selection.Y, W: selection.W, H: 1}, th.Selection)
		ui.Rect(mathx.Rect{X: selection.X, Y: selection.Y + selection.H - 1, W: selection.W, H: 1}, th.Selection)
	}
	if v.Frames <= 0 || r.W <= 0 {
		return
	}
	for i, hx := range xs {
		frame := ls
		if i == 1 {
			frame = le
		}
		visible := frame >= start && frame <= end
		// An already captured handle must continue receiving deltas and its
		// release even after it is dragged beyond the visible range.
		if !visible && ui.Active() != hid[i] {
			continue
		}
		hr := mathx.Rect{X: hx - handleW/2, Y: r.Y, W: handleW, H: r.H}
		if d, ok := ui.Drag(hid[i], hr); ok {
			if p.clip.drag != i {
				p.clip.drag, p.clip.start, p.clip.end, p.clip.acc = i, ls, le, 0
				p.clip.loopSettings = v.Settings.Clone()
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
		if !visible {
			continue
		}
		col := th.AccentDim
		if ui.Active() == hid[i] || ui.Hot() == hid[i] {
			col = th.Accent
		}
		ui.Rect(mathx.Rect{X: hx - 1, Y: r.Y, W: 2, H: r.H}, col)
		ui.Rect(mathx.Rect{X: hx - handleW/2, Y: r.Y, W: handleW, H: 6}, col)
	}
	if v.WaveNav || v.WaveSelect {
		id := ui.ID("wave-selection")
		if _, active := ui.Drag(id, r); active {
			if frame, ok := audioWaveFrame(ui.Mouse().X, r, start, end); ok {
				if !p.clip.selecting {
					p.clip.selectAnchor = frame
				}
				p.clip.selectEnd = frame
				p.clip.selecting = ui.Active() == id
			}
		} else {
			p.clip.selecting = false
		}
	} else {
		p.clip.selecting = false
	}
	if v.Playing && v.Rate > 0 {
		position := float64(v.Position) * float64(v.Rate)
		if position >= float64(start) && position <= float64(end) {
			px := r.X + float32(position-float64(start))*k
			ui.Rect(mathx.Rect{X: px, Y: r.Y, W: 1, H: r.H}, th.Text)
		}
	}
}

func stereoWaveRect(r mathx.Rect, cols, col, channel int, amplitude float32) (mathx.Rect, bool) {
	if r.W <= 0 || r.H <= 0 || cols <= 0 || col < 0 || col >= cols || channel < 0 || channel >= 2 ||
		math.IsNaN(float64(amplitude)) || math.IsInf(float64(amplitude), 0) {
		return mathx.Rect{}, false
	}
	w := r.W / float32(cols)
	x := r.X + float32(col)*w
	width := min(max(w, 1), r.X+r.W-x)
	half := min(max(min(max(amplitude, 0), 1)*max(r.H/4-3, 0), .5), r.H/4)
	center := r.Y + (float32(channel)+.5)*r.H/2
	return mathx.Rect{X: x, Y: center - half, W: max(width, 0), H: 2 * half}, true
}

func (p *plugin) commitLoop(ed *context.Editor, v ClipView) {
	start, end := p.clip.start, p.clip.end
	before := p.clip.loopSettings
	p.clip.drag = -1
	p.clip.loopSettings = nil
	if ls, le := loopOf(v); ls == start && le == end {
		return
	}
	if before == nil {
		// There was no captured handle gesture; never infer an old preimage.
		return
	}
	n := before.Clone()
	n["loopStart"], n["loopEnd"] = aetext.Int(start), aetext.Int(end)
	p.write(ed, before, n)
}

func (p *plugin) write(ed *context.Editor, old, n pipeline.Settings) {
	d, err := ed.DocumentForAsset(p.clip.id)
	if err != nil {
		p.notify(err)
		return
	}
	if d.Metadata == nil || d.Metadata.ReadOnly || d.Metadata.Conflict {
		p.notify(fmt.Errorf("resolve the audio metadata conflict before editing"))
		return
	}
	m, err := asset.ParseSourceMeta(d.Metadata.Code.Bytes(), d.Metadata.Path)
	if err != nil {
		p.notify(err)
		return
	}
	changed, err := mergeClipSettings(m.Settings, old, n)
	if err != nil {
		p.notify(err)
		return
	}
	if changed {
		// Keep source identity, frames and unknown metadata. All changed fields
		// are applied as one step in the existing shared metadata history.
		d.Metadata.Code.ReplaceSnapshot(asset.MarshalSourceMeta(m), false)
	}
}

// The Inspector and a centre tab may edit the same metadata in one frame.
// Change only fields touched by this control and reject a stale write to the
// same field. Validate all fields before mutating the parsed snapshot.
func mergeClipSettings(current, before, after pipeline.Settings) (bool, error) {
	keys := []string{"loopStart", "loopEnd", "stream", "gain", "bus"}
	var changed []string
	for _, key := range keys {
		old, oldOK := before[key]
		next, nextOK := after[key]
		if equalClipSetting(old, oldOK, next, nextOK) {
			continue
		}
		now, nowOK := current[key]
		if !equalClipSetting(old, oldOK, now, nowOK) {
			return false, fmt.Errorf("%s", i18n.Tr("editor.audio.setting_changed", "This audio setting changed in another view. Review it before editing again."))
		}
		changed = append(changed, key)
	}
	if slices.Contains(changed, "loopStart") || slices.Contains(changed, "loopEnd") {
		// The handles are constrained as a pair. A concurrently moved opposite
		// handle can invalidate this drag even if its own field was not edited.
		for _, key := range []string{"loopStart", "loopEnd"} {
			old, oldOK := before[key]
			now, nowOK := current[key]
			if !equalClipSetting(old, oldOK, now, nowOK) {
				return false, fmt.Errorf("%s", i18n.Tr("editor.audio.setting_changed", "This audio setting changed in another view. Review it before editing again."))
			}
		}
	}
	for _, key := range changed {
		if value, ok := after[key]; ok {
			current[key] = value.Clone()
		} else {
			delete(current, key)
		}
	}
	return len(changed) != 0, nil
}

func equalClipSetting(a aetext.Value, aOK bool, b aetext.Value, bOK bool) bool {
	if aOK != bOK {
		return false
	}
	if !aOK {
		return true
	}
	// Serialization may parse 1.0 as integer 1. Limit cross-kind comparison
	// to exactly representable integers rather than rounding large frame values.
	if a.Kind == aetext.VInt && b.Kind == aetext.VFloat {
		return a.Int >= -(1<<53) && a.Int <= 1<<53 && float64(a.Int) == b.Float
	}
	if a.Kind == aetext.VFloat && b.Kind == aetext.VInt {
		return b.Int >= -(1<<53) && b.Int <= 1<<53 && a.Float == float64(b.Int)
	}
	if a.Kind == aetext.VFloat && b.Kind == aetext.VFloat {
		return a.Float == b.Float
	}
	return reflect.DeepEqual(a, b)
}

func previewPlayLabel(v ClipView) string {
	play := i18n.Tr("i18n.audio.play", "Play")
	if v.Preparing {
		return i18n.Tr("editor.audio.preview_cancel", "Cancel")
	}
	if v.Playing {
		play = i18n.Tr("i18n.audio.pause", "Pause")
		if v.Paused {
			play = i18n.Tr("i18n.audio.resume", "Resume")
		}
	}
	return play
}

func (p *plugin) transport(ed *context.Editor, mod *audio.Module, v ClipView) {
	ui := ed.UI
	owner := p.audioPreviewOwner()
	owner.pollWorkingAudio(mod)
	v.Preparing = owner.workingAudioPending(v.ID)
	play := previewPlayLabel(v)
	stop := i18n.Tr("i18n.audio.stop", "Stop")
	loop := i18n.Tr("i18n.audio.loop", "Loop")
	// Reserve room for all three play states so pausing does not move the
	// controls. Narrow panels wrap buttons rather than overlapping translations.
	playWidth := max(ui.TextSize(i18n.Tr("i18n.audio.play", "Play")).X,
		ui.TextSize(i18n.Tr("i18n.audio.pause", "Pause")).X,
		ui.TextSize(i18n.Tr("i18n.audio.resume", "Resume")).X,
		ui.TextSize(i18n.Tr("editor.audio.preview_cancel", "Cancel")).X)
	row := ui.Cursor().Next(0)
	x := row.X
	at := func(textWidth float32) mathx.Rect {
		w := min(max(row.W, 0), textWidth+2*ui.Theme.Padding)
		if x > row.X && x+w > row.X+row.W {
			row = ui.Cursor().Next(0)
			x = row.X
		}
		r := mathx.Rect{X: x, Y: row.Y, W: w, H: row.H}
		x += w + 4
		return r
	}
	fit := func(label string, r mathx.Rect) string {
		width := max(r.W-2*ui.Theme.Padding, 0)
		if ui.TextSize(label).X > width && ui.TextSize("…").X > width {
			return ""
		}
		return widgets.Elide(ui, label, width)
	}
	playRect := at(playWidth)
	playID := ui.ID("play")
	clickedPlay := widgets.ButtonRect(ui, playID, playRect, fit(play, playRect))
	widgets.Tooltip(ui, playID, play)
	if clickedPlay && mod != nil {
		if v.Preparing {
			owner.cancelWorkingAudio()
		} else if v.Playing {
			owner.cancelWorkingAudio()
			p.notify(mod.SetPreviewPaused(!v.Paused))
		} else {
			var d *document.AssetDocument
			if ed.Docs != nil {
				d = ed.Docs.AssetDocument(v.ID)
			}
			if d != nil && d.Metadata != nil && d.Metadata.Dirty() {
				p.notify(owner.prepareWorkingAudio(ed, mod, d, p.clip.loop))
			} else {
				owner.cancelWorkingAudio()
				err := localizeAudioSeekError(mod.Preview(ed.Loader, v.ID, p.clip.loop))
				if err == nil || mod.PreviewClip().IsZero() {
					owner.aud = audition{}
				}
				p.notify(err)
			}
		}
	}
	stopRect := at(ui.TextSize(stop).X)
	stopID := ui.ID("stop")
	clickedStop := widgets.ButtonRect(ui, stopID, stopRect, fit(stop, stopRect))
	widgets.Tooltip(ui, stopID, stop)
	if clickedStop {
		owner.cancelWorkingAudio()
		if mod != nil {
			p.notify(mod.StopPreview())
		}
	}
	loopRect := at(ui.TextSize(loop).X)
	loopID := ui.ID("looping")
	if widgets.ToggleButton(ui, loopID, loopRect, fit(loop, loopRect), &p.clip.loop) && v.Preparing {
		owner.cancelWorkingAudio()
	}
	widgets.Tooltip(ui, loopID, loop)
	if v.Preparing {
		widgets.LabelWrapped(ui, i18n.Tr("editor.audio.preview_preparing", "Preparing preview..."), true)
	} else if v.Playing && mod != nil && mod.PreviewStreamSeekPending() {
		widgets.LabelWrapped(ui, i18n.Tr("i18n.audio.seek_preparing", "Seeking preview..."), true)
	} else {
		widgets.LabelWrapped(ui, i18n.Tr("i18n.audio.position_s_of_seconds_s", "{position} s of {seconds} s", i18n.Str("position", seconds(v.Position)), i18n.Str("seconds", seconds(v.Seconds))), true)
	}
}

func (p *plugin) importFields(ed *context.Editor, s pipeline.Settings) {
	ui := ed.UI
	widgets.LabelDim(ui, i18n.Tr("i18n.audio.import", "Import"))
	cur := streamMode(s)
	r := widgets.Field(ui, i18n.Tr("i18n.audio.stream", "Stream"))
	w := max(0, (r.W-8)/3)
	for i, m := range streamModes {
		on := cur == m[0]
		br := mathx.Rect{X: r.X + float32(i)*(w+4), Y: r.Y, W: w, H: r.H}
		if widgets.ToggleButton(ui, core.IDSeq(ui.ID("stream"), i), br, streamLabels()[i], &on) && cur != m[0] {
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
		if !p.clip.gainOn {
			p.clip.gainSettings = s.Clone()
		}
		p.clip.gain, p.clip.gainOn = min(max(g, 0), maxGain), true
	}
	if e.End && p.clip.gainOn {
		p.clip.gainOn = false
		before := p.clip.gainSettings
		p.clip.gainSettings = nil
		if before != nil && p.clip.gain != s.Float("gain", 1) {
			n := before.Clone()
			n["gain"] = aetext.Float64(p.clip.gain)
			p.write(ed, before, n)
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
	d, err := ed.DocumentForAsset(p.clip.id)
	if err != nil || d.Metadata == nil {
		return
	}
	undo, redo := twoButtons(ui)
	if widgets.ButtonRect(ui, ui.ID("undo"), undo, i18n.Tr("i18n.audio.undo", "Undo")) {
		p.notify(ed.UndoAsset(d))
	}
	if widgets.ButtonRect(ui, ui.ID("redo"), redo, i18n.Tr("i18n.audio.redo", "Redo")) {
		p.notify(ed.RedoAsset(d))
	}
	p.metadataConflict(ed)
	if d.Dirty() && widgets.Button(ui, i18n.Tr("editor.document.save_metadata", "Save Metadata")) {
		p.notify(ed.SaveAsset(d))
	}
}

func (p *plugin) metadataConflict(ed *context.Editor) {
	ui := ed.UI
	d, err := ed.DocumentForAsset(p.clip.id)
	if err != nil || d.Metadata == nil {
		return
	}
	if d.Metadata.Conflict {
		widgets.LabelWrapped(ui, i18n.Tr("editor.document.metadata_conflict", "Metadata changed on disk. Reload it or explicitly keep your changes before saving."), true)
		if widgets.Button(ui, i18n.Tr("editor.scripteditor.reload", "Reload")) {
			p.notify(ed.Docs.ReloadScript(d.Metadata))
		}
		if widgets.Button(ui, i18n.Tr("editor.scripteditor.keep_mine", "Keep mine")) {
			ed.Docs.KeepScript(d.Metadata)
		}
	}
}
