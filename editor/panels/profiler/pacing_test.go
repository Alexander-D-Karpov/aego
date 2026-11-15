package profiler

import (
	"fmt"
	"math"
	"testing"
	"time"

	"aego/core/stats"
	"aego/core/trace"
	"aego/editor/core/context"
	"aego/editor/core/play"
	"aego/render"
)

func TestFPSUsesMeasuredFrameIntervals(t *testing.T) {
	for _, fps := range []float64{7.3, 15, 60} {
		for interval, seconds := range intervals() {
			t.Run(fmt.Sprintf("%.1f_interval_%d_%gs", fps, interval, seconds), func(t *testing.T) {
				p := New(Deps{})
				p.interval = interval
				// Simulated frame time deliberately differs from wall-clock sampling.
				f := &stats.Frame{FrameTime: 1.0 / 144}
				for i := 0; i < 1000 && p.n < 2; i++ {
					p.Record(100+float64(i)/fps, f, render.Stats{}, play.Stats{}, context.ScriptStats{}, false, false)
					if p.n > 0 {
						got := p.at(p.n - 1).fps()
						if math.Abs(got-fps) > 0.00001 {
							t.Fatalf("measured %.5f FPS, want %.5f", got, fps)
						}
					}
				}
				if p.n < 2 {
					t.Fatalf("completed %d buckets, want at least two", p.n)
				}
			})
		}
	}
}

func TestFPSDoesNotCountProfilerPauseAsOneFrame(t *testing.T) {
	p := New(Deps{})
	f := &stats.Frame{FrameTime: 0.1}
	record := func(now float64) {
		p.Record(now, f, render.Stats{}, play.Stats{}, context.ScriptStats{}, false, false)
	}
	for _, now := range []float64{10, 10.1, 10.2, 10.3} {
		record(now)
	}
	p.paused = true
	record(20)
	p.paused = false
	for _, now := range []float64{30, 30.1, 30.2, 30.3} {
		record(now)
	}
	if p.n < 2 || math.Abs(p.at(p.n-1).fps()-10) > 0.00001 {
		t.Fatalf("resumed FPS: buckets=%d fps=%v", p.n, p.at(p.n-1).fps())
	}
}

func TestFrameRatingExcludesOnlyPacingWait(t *testing.T) {
	ms := func(n int64) int64 { return n * int64(time.Millisecond) }
	for _, tc := range []struct {
		name  string
		frame trace.Frame
		want  frameRating
	}{
		{"idle", trace.Frame{Dur: ms(67), PaceWaitNS: ms(65), IdleWaitNS: ms(65)}, frameIdle},
		{"idle work within idle budget", trace.Frame{Dur: ms(100), PaceWaitNS: ms(60), IdleWaitNS: ms(60)}, frameIdle},
		{"idle real work warning", trace.Frame{Dur: ms(130), PaceWaitNS: ms(60), IdleWaitNS: ms(60)}, frameWarn},
		{"idle real work error", trace.Frame{Dur: ms(210), PaceWaitNS: ms(60), IdleWaitNS: ms(60)}, frameError},
		{"unexplained slow frame", trace.Frame{Dur: ms(67)}, frameError},
		{"normal pacing", trace.Frame{Dur: ms(17), PaceWaitNS: ms(15)}, frameOK},
		{"out of range wait is clamped", trace.Frame{Dur: ms(5), PaceWaitNS: ms(7)}, frameOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := rateFrame(&tc.frame); got != tc.want {
				t.Fatalf("rating %v, want %v", got, tc.want)
			}
		})
	}
}
