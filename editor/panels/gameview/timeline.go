package gameview

import (
	"strconv"

	"aego/core/mathx"
	"aego/editor/core/play"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

// Scrubber is the part of the play controller the timeline bar drives.
type Scrubber interface {
	Timeline() play.Timeline
	State() play.State
	Ticks() uint64
	Seek(n int) error
	Scrub(n int) error
}

type TimelineBar struct {
	src      Scrubber
	bar      mathx.Rect
	dragging bool
	dragTo   int
	pending  int
	sought   int
	release  bool
	seekErr  string
}

func NewTimelineBar(src Scrubber) *TimelineBar {
	return &TimelineBar{src: src, pending: -1, sought: -1}
}

func (b *TimelineBar) Reset() {
	b.bar, b.dragging, b.pending, b.sought, b.release = mathx.Rect{}, false, -1, -1, false
}

// hold records where the drag is; the seek waits for Apply, so any number of mouse moves in a frame cost one seek.
func (b *TimelineBar) hold(n int, down bool) {
	b.dragTo, b.pending = n, n
	if !down {
		b.dragging, b.release = false, true
	}
}

// Apply runs at most one seek per frame: a scrub to the latest drag position, or the final seek after release.
func (b *TimelineBar) Apply() {
	if b.pending < 0 {
		return
	}
	n := b.pending
	if b.release {
		b.release, b.pending = false, -1
		if n != b.sought && b.seekErr == "" {
			if err := b.src.Seek(n); err != nil {
				b.seekErr = err.Error()
			}
		}
		b.sought = -1
		return
	}
	if n == b.sought || b.seekErr != "" {
		return
	}
	if tl := b.src.Timeline(); tl.KeysOff != "" && n < tl.Cursor {
		return
	}
	b.sought = n
	if err := b.src.Scrub(n); err != nil {
		b.seekErr = err.Error()
	}
}

func (b *TimelineBar) Clear() {
	b.bar = mathx.Rect{}
}

func (b *TimelineBar) Holds(m mathx.Vec2) bool {
	return b.dragging || b.bar.Contains(m)
}

func megabytes(b int) string {
	return strconv.FormatFloat(float64(b)/(1<<20), 'f', 1, 64) + " MB"
}

func (b *TimelineBar) idle(ui *core.Context, bar mathx.Rect, tl play.Timeline) {
	th := &ui.Theme
	dot := mathx.Rect{X: bar.X + 10, Y: bar.Y + bar.H/2 - 4, W: 8, H: 8}
	ui.Box(dot, 4, th.Widget, th.TextDim)
	msg := i18n.Tr("editor.gameview.recording_starts_with_the_first", "Recording starts with the first frame · press Play or Step")
	if tl.Why != "" {
		msg = i18n.Tr("editor.gameview.history_cleared_why_press_play", "History cleared: {why} · press Play or Step to record again", i18n.Str("why", tl.Why))
	}
	right := "tick " + strconv.FormatUint(b.src.Ticks(), 10)
	switch {
	case tl.KeysOff != "":
		right = "keyframes off: " + tl.KeysOff + " · " + right
	case tl.Keys > 0:
		right = strconv.Itoa(tl.Keys) + " key, " + megabytes(tl.KeyBytes) + " · " + right
	}
	rw := min(ui.TextSize(right).X+12, bar.W*0.4)
	tx := dot.X + dot.W + 8
	ui.PushClip(mathx.Rect{X: tx, Y: bar.Y, W: max(0, bar.X+bar.W-rw-tx-8), H: bar.H})
	ui.TextIn(mathx.Rect{X: tx, Y: bar.Y, W: bar.W, H: bar.H}, msg, th.TextDim, core.AlignLeft)
	ui.PopClip()
	ui.TextIn(mathx.Rect{X: bar.X + bar.W - rw - 6, Y: bar.Y, W: rw, H: bar.H}, right, th.TextDisabled, core.AlignRight)
}

func (b *TimelineBar) Draw(ui *core.Context, r mathx.Rect) {
	tl := b.src.Timeline()
	if b.src.State() == play.Stopped && tl.Blocked == "" {
		return
	}
	th := &ui.Theme
	bar := mathx.Rect{X: r.X + 6, Y: r.Y + r.H - th.BarHeight - 6, W: max(0, r.W-12), H: th.BarHeight}
	b.bar = bar
	ui.PushClip(r)
	defer ui.PopClip()
	ui.Rect(bar, th.PanelAlt)
	ui.Outline(bar, th.BarBorder)
	if tl.Blocked != "" {
		ui.TextIn(bar, tl.Blocked, th.TextDim, core.AlignCenter)
		return
	}
	if tl.Frames == 0 {
		b.idle(ui, bar, tl)
		return
	}
	type part struct {
		text string
		col  mathx.Color
	}
	parts := []part{{"tick " + strconv.FormatUint(tl.Tick, 10), th.Text}, {"  " + strconv.Itoa(tl.Cursor) + " / " + strconv.Itoa(tl.Frames), th.TextDim}}
	switch {
	case b.seekErr != "":
		parts = []part{{b.seekErr, th.Error}}
	case tl.Why != "":
		parts = append(parts, part{"  · ended: " + tl.Why, th.Warn})
	case tl.KeysOff != "":
		parts = append(parts, part{"  · keyframes off: " + tl.KeysOff + ", rewinding when released", th.TextDisabled})
	case tl.Keys > 0:
		parts = append(parts, part{"  · " + strconv.Itoa(tl.Keys) + " keys, " + megabytes(tl.KeyBytes), th.TextDisabled})
	}
	full := float32(0)
	for _, pt := range parts {
		full += ui.TextSize(pt.text).X
	}
	lw := min(full+16, bar.W*0.45)
	track := mathx.Rect{X: bar.X + 10, Y: bar.Y + bar.H/2 - 3, W: max(1, bar.W-lw-24), H: 6}
	total := max(tl.Frames, 1)
	xAt := func(n int) float32 { return track.X + track.W*float32(n)/float32(total) }
	frameAt := func(x float32) int {
		return int(mathx.Clamp((x-track.X)/track.W, 0, 1)*float32(total) + 0.5)
	}
	ui.Rect(track, th.Widget)
	lo := xAt(tl.Floor)
	ui.Rect(mathx.Rect{X: lo, Y: track.Y, W: max(0, xAt(tl.Usable)-lo), H: track.H}, th.AccentDim)
	id := ui.ID("timeline")
	hit := mathx.Rect{X: track.X - 4, Y: bar.Y, W: track.W + 8, H: bar.H}
	if ui.Pressed(id, hit) {
		b.dragging, b.seekErr, b.sought, b.release = true, "", -1, false
	}
	cur := tl.Cursor
	if tl.Beyond {
		cur = tl.Frames
	}
	if b.dragging {
		b.hold(min(max(frameAt(ui.Mouse().X), tl.Floor), tl.Usable), ui.MouseDown(input.MouseLeft))
	}
	if b.dragging || b.pending >= 0 {
		cur = b.dragTo
	}
	hx := xAt(cur)
	ui.Rect(mathx.Rect{X: hx - 2, Y: bar.Y + 3, W: 4, H: bar.H - 6}, th.Accent)
	lr := mathx.Rect{X: track.X + track.W + 10, Y: bar.Y, W: lw - 4, H: bar.H}
	ui.PushClip(lr)
	x := lr.X
	for _, pt := range parts {
		ui.Text(pt.text, mathx.Vec2{X: x, Y: ui.TextY(lr)}, pt.col)
		x += ui.TextSize(pt.text).X
	}
	ui.PopClip()
	tip := i18n.Tr("editor.gameview.drag_to_rewind_the_game", "Drag to rewind the game; Ctrl+Left and Ctrl+Right step one frame")
	if tl.Keys > 0 {
		tip += i18n.Tr("editor.gameview.keys_keyframes_keybytes_the_last", "\n{keys} keyframes, {keybytes}; the last took {lastkey} and {formatfloat} ms", i18n.Str("keys", strconv.Itoa(tl.Keys)), i18n.Str("keybytes", megabytes(tl.KeyBytes)), i18n.Str("lastkey", megabytes(tl.LastKey)), i18n.Str("formatfloat", strconv.FormatFloat(float64(tl.KeyTime.Microseconds())/1000, 'f', 2, 64)))
		tip += i18n.Tr("editor.gameview.a_key_every_formatuint_ticks", "\na key every {formatuint} ticks", i18n.Str("formatuint", strconv.FormatUint(tl.KeyEvery, 10)))
		if tl.Cached > 0 {
			tip += i18n.Tr("editor.gameview.cached_cached_for_scrubbing_cachebytes", "; {cached} cached for scrubbing, {cachebytes}", i18n.Str("cached", strconv.Itoa(tl.Cached)), i18n.Str("cachebytes", megabytes(tl.CacheBytes)))
		}
	}
	if tl.Why != "" {
		tip += i18n.Tr("editor.gameview.history_ends_why", "\nHistory ends: {why}", i18n.Str("why", tl.Why))
	}
	widgets.Tooltip(ui, id, tip)
}
