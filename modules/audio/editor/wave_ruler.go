package editor

import (
	"math"
	"strconv"

	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/ui/core"
)

const (
	audioRulerGap      = 72
	audioRulerMaxTicks = 32
)

type audioRulerTick struct {
	X       float32
	Seconds float64
}

// audioRulerTicks uses source time, not pyramid bucket indices. Its bounded
// result is only a visual guide; seeking and sample editing are not implied.
func audioRulerTicks(start, end int64, rate int, width float32) ([audioRulerMaxTicks]audioRulerTick, int, float64) {
	var ticks [audioRulerMaxTicks]audioRulerTick
	if start < 0 || end <= start || end > 1<<53 || rate <= 0 || width <= 0 ||
		math.IsNaN(float64(width)) || math.IsInf(float64(width), 0) {
		return ticks, 0, 0
	}
	span := float64(end - start)
	gap := max(float64(audioRulerGap), float64(width)/float64(audioRulerMaxTicks-2))
	ideal := max(span/float64(rate)*gap/float64(width), 1/float64(rate))
	base := math.Pow(10, math.Floor(math.Log10(ideal)))
	step := 10 * base
	for _, multiplier := range [...]float64{1, 2, 5, 10} {
		if multiplier*base >= ideal {
			step = multiplier * base
			break
		}
	}
	first := math.Ceil(float64(start)/float64(rate)/step - 1e-10)
	n := 0
	for i := 0; i < audioRulerMaxTicks; i++ {
		seconds := (first + float64(i)) * step
		frame := seconds*float64(rate) - float64(start)
		// Roundoff at a tick exactly on the viewport boundary is harmless;
		// never display a tick which is genuinely outside the visible range.
		if frame > span+1e-4 {
			break
		}
		if frame < -1e-4 {
			continue
		}
		ticks[n] = audioRulerTick{X: float32(min(max(frame/span, 0), 1) * float64(width)), Seconds: seconds}
		n++
	}
	return ticks, n, step
}

func audioRulerLabel(seconds, step float64) string {
	precision := 0
	if step < 1 {
		precision = min(max(int(math.Ceil(-math.Log10(step)-1e-10)), 0), 9)
	}
	return i18n.Tr("i18n.audio.wave_time", "{time} s", i18n.Str("time", strconv.FormatFloat(seconds, 'f', precision, 64)))
}

func drawAudioRuler(ui *core.Context, v ClipView) {
	r := ui.Cursor().Next(ui.Theme.RowHeight)
	start, end := audioWaveBounds(v.WaveStart, v.WaveEnd, v.Frames)
	ticks, n, step := audioRulerTicks(start, end, v.Rate, r.W)
	if n == 0 || r.H <= 0 {
		return
	}
	lineY := r.Y + r.H - 1
	ui.Rect(mathx.Rect{X: r.X, Y: lineY, W: r.W, H: 1}, ui.Theme.Separator)
	lastRight := r.X - 4
	for _, tick := range ticks[:n] {
		x := min(r.X+tick.X, r.X+r.W-1)
		ui.Rect(mathx.Rect{X: x, Y: lineY - 4, W: 1, H: 5}, ui.Theme.TextDim)
		label := audioRulerLabel(tick.Seconds, step)
		size := ui.TextSize(label)
		if size.X <= 0 || size.X > r.W {
			continue
		}
		left := min(max(x-size.X/2, r.X), r.X+r.W-size.X)
		if left < lastRight+4 {
			continue
		}
		ui.Text(label, mathx.Vec2{X: left, Y: ui.TextY(mathx.Rect{Y: r.Y, H: max(r.H-5, 0)})}, ui.Theme.TextDim)
		lastRight = left + size.X
	}
}
