//go:build aego_golden

package golden

import (
	"testing"
	"time"

	"aego/core/mathx"
	"aego/editor/core/play"
	"aego/editor/panels/gameview"
)

type fakeScrub struct {
	tl    play.Timeline
	ticks uint64
}

func (f *fakeScrub) Timeline() play.Timeline { return f.tl }
func (f *fakeScrub) State() play.State       { return play.Paused }
func (f *fakeScrub) Ticks() uint64           { return f.ticks }
func (f *fakeScrub) Seek(int) error          { return nil }
func (f *fakeScrub) Scrub(int) error         { return nil }

func TestGameViewTimeline(t *testing.T) {
	env := newEditorEnvSize(t, 800, 200)
	bars := []*gameview.TimelineBar{
		gameview.NewTimelineBar(&fakeScrub{tl: play.Timeline{Keys: 1, KeyBytes: 60 << 10}}),
		gameview.NewTimelineBar(&fakeScrub{tl: play.Timeline{Why: "assets were reloaded during Play"}, ticks: 420}),
		gameview.NewTimelineBar(&fakeScrub{tl: play.Timeline{Frames: 2759, Cursor: 1800, Usable: 2759, Floor: 300, Keys: 7,
			KeyBytes: 420 << 10, LastKey: 64 << 10, KeyTime: 140 * time.Microsecond, Tick: 1800}}),
		gameview.NewTimelineBar(&fakeScrub{tl: play.Timeline{Frames: 900, Cursor: 900, Usable: 600, Why: "the scene was edited during Play", KeysOff: "opaque.state", Tick: 900}}),
	}
	img, err := env.draw(func(full mathx.Rect) {
		h := full.H / float32(len(bars))
		for i, b := range bars {
			b.Draw(env.ui, mathx.Rect{X: full.X, Y: full.Y + h*float32(i), W: full.W, H: h})
		}
	})
	checkGolden(t, "gameview_timeline", img, err)
}
