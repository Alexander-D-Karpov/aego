package integration

import (
	"slices"
	"testing"
	"time"

	"aego/app/run"
	"aego/core/rewind"
	"aego/engine"
	"aego/modules/animation"
	"aego/project"
	"aego/sdk"
)

func platformerProject(t *testing.T) *project.Project {
	t.Helper()
	p, err := project.Load(copyExample(t, "platformer"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDeterminismPlatformer(t *testing.T) {
	rep, err := run.Determinism(platformerProject(t), run.DeterminismOptions{Frames: 240, Every: 30}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Diverged != 0 {
		t.Fatalf("two runs diverge by frame %d", rep.Diverged)
	}
	for _, id := range []string{"scene", "script.runtime", "physics.sim", animation.ServiceID} {
		if !slices.Contains(rep.Covered, id) {
			t.Fatalf("hash does not cover %s: %v", id, rep.Covered)
		}
	}
}

type wallModule struct{}

type wallClock struct {
	ns int64
}

type wallKey struct {
	ns int64
}

func (k *wallKey) Size() int {
	return 8
}

func (c *wallClock) Capture(reuse rewind.Key) rewind.Key {
	return &wallKey{ns: c.ns}
}

func (c *wallClock) Restore(k rewind.Key) {
	c.ns = k.(*wallKey).ns
}

func (c *wallClock) Hash(h *rewind.Hasher) {
	h.I64(c.ns)
}

type wallTick struct{}

func (wallTick) Run(ctx *engine.Context) {
	if c, ok := engine.ServiceOf[*wallClock](ctx.World, "wall.clock"); ok {
		c.ns = time.Now().UnixNano()
	}
}

func (wallModule) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "wall", Version: "0.1.0"}
}

func (wallModule) Register(r *sdk.Registry) error {
	r.System(engine.SystemDesc{ID: "wall.tick", Phase: engine.PhaseUpdate, Scope: engine.ScopePlay, System: wallTick{}})
	return nil
}

func (wallModule) AttachWorld(ctx *sdk.AttachContext) error {
	if ctx.Play && ctx.World != nil {
		ctx.World.SetService("wall.clock", &wallClock{})
	}
	return nil
}

func (wallModule) DetachWorld(ctx *sdk.AttachContext) {
	if ctx.Play && ctx.World != nil {
		ctx.World.RemoveService("wall.clock")
	}
}

func TestDeterminismCatchesWallClock(t *testing.T) {
	rep, err := run.Determinism(platformerProject(t), run.DeterminismOptions{Frames: 60, Every: 10}, combatModule{}, wallModule{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Diverged == 0 || rep.Diverged > 10 {
		t.Fatalf("diverged at %d, want the first checkpoint", rep.Diverged)
	}
}
