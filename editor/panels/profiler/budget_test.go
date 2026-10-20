package profiler

import (
	"testing"

	"aego/core/stats"
	"aego/editor/core/context"
)

type notices struct{ n int }

func (n *notices) Notify(context.Level, string) { n.n++ }
func (n *notices) Ask(context.Question)         {}

func TestBudgetIgnoresThrottledFrames(t *testing.T) {
	for _, tc := range []struct {
		throttled bool
		want      int
	}{{false, 1}, {true, 0}} {
		n := &notices{}
		p := New(Deps{Notices: n, Throttled: func() bool { return tc.throttled }})
		f := &stats.Frame{FrameTime: 1.0 / 15}
		for i := range 3 {
			p.budget(10+float64(i), f)
		}
		if n.n != tc.want {
			t.Fatalf("throttled=%v: %d notices, want %d", tc.throttled, n.n, tc.want)
		}
	}
}
