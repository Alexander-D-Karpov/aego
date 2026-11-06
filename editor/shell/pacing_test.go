package shell

import (
	"testing"
	"time"

	"aego/core/log"
	"aego/editor/core/context"
	"aego/editor/core/play"
	"aego/editor/core/tasks"
	"aego/editor/core/workspace"
	"aego/engine"
	"aego/ui/core"
	"aego/ui/dnd"
)

func TestPaceFrameRecordsIdleAndActiveWait(t *testing.T) {
	e, err := engine.NewBuilder(engine.Config{Headless: true, Trace: true, ResRoot: t.TempDir(), UserRoot: t.TempDir()}).WithLogger(log.Nop()).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	ctx := &context.Editor{DnD: &dnd.State{}}
	a := &App{opt: Options{Paced: true}, eng: e, ctx: ctx, play: play.New(ctx), ws: workspace.New("", nil), tasks: tasks.New(tasks.Deps{}), ui: core.New(nil)}
	for _, tc := range []struct {
		name    string
		idle    bool
		zone    string
		minimum time.Duration
	}{
		{"idle", true, engine.ZoneIdleWait, time.Second / 15},
		{"active", false, engine.ZoneFrameWait, time.Second / 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.lastInput = e.Platform().Now()
			if tc.idle {
				a.lastInput -= 2
			}
			// Leave extra margin so scheduler delays before the call don't skip the wait.
			a.lastFrame = time.Now().Add(100 * time.Millisecond)
			tr := e.Trace()
			tr.BeginFrame()
			a.PaceFrame()
			tr.EndFrame()
			f := tr.Frame(0)
			if f.PaceWaitNS < int64(tc.minimum) || (f.IdleWaitNS > 0) != tc.idle {
				t.Fatalf("pace wait=%d idle wait=%d", f.PaceWaitNS, f.IdleWaitNS)
			}
			if len(f.Events) != 1 || tr.Name(f.Events[0].Zone) != tc.zone {
				t.Fatalf("wait trace: %+v", f.Events)
			}
		})
	}
}
