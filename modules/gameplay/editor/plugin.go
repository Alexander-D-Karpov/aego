package editor

import (
	"errors"

	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/ext"
	"aego/editor/ext/inspect"
	"aego/modules/gameplay"
	"aego/scene"
)

const (
	CmdAreas = gameplay.ID + ".areas"
	partBase = 60
	minSize  = float32(0.1)
)

var (
	colHit     = mathx.Color{R: 0.95, G: 0.3, B: 0.3, A: 0.9}
	colHurt    = mathx.Color{R: 0.3, G: 0.9, B: 0.45, A: 0.9}
	colKill    = mathx.Color{R: 0.85, G: 0.2, B: 0.6, A: 0.9}
	colCheck   = mathx.Color{R: 0.35, G: 0.7, B: 1, A: 0.9}
	colPick    = mathx.Color{R: 1, G: 0.8, B: 0.2, A: 0.9}
	colDim     = mathx.Color{R: 0.7, G: 0.7, B: 0.7, A: 0.3}
	colHealth  = mathx.Color{R: 0.3, G: 0.85, B: 0.4, A: 1}
	colHurtBar = mathx.Color{R: 0.85, G: 0.25, B: 0.25, A: 1}
)

type plugin struct {
	ed   *context.Editor
	prof profileState
}

func reader[T any](get func(*T) mathx.Rect) func(*scene.Scene, scene.NodeHandle) (mathx.Rect, bool) {
	return func(s *scene.Scene, h scene.NodeHandle) (mathx.Rect, bool) {
		c, ok := scene.Get[T](s, h)
		if !ok {
			return mathx.Rect{}, false
		}
		return get(c), true
	}
}

func Hook(e *ext.Registry) error {
	p := &plugin{ed: e.Editor()}
	sr := e.Schema()
	var errs []error
	for _, a := range []struct {
		name string
		col  mathx.Color
		read func(*scene.Scene, scene.NodeHandle) (mathx.Rect, bool)
	}{
		{gameplay.NameHitbox, colHit, reader(func(c *gameplay.Hitbox2D) mathx.Rect { return c.Area })},
		{gameplay.NameHurtbox, colHurt, reader(func(c *gameplay.Hurtbox2D) mathx.Rect { return c.Area })},
		{gameplay.NameKillZone, colKill, reader(func(c *gameplay.KillZone2D) mathx.Rect { return c.Area })},
		{gameplay.NameCheckpoint, colCheck, reader(func(c *gameplay.Checkpoint2D) mathx.Rect { return c.Area })},
		{gameplay.NamePickup, colPick, reader(func(c *gameplay.Pickup2D) mathx.Rect { return c.Area })},
	} {
		td, ok := sr.ByName(a.name)
		if !ok {
			continue
		}
		g := &areaGizmo{td: td, col: a.col, read: a.read, active: -1}
		if a.name == gameplay.NameHitbox {
			g.extra = drawKnockback
		}
		errs = append(errs, e.Gizmo(a.name, g))
	}
	errs = append(errs,
		e.Inspector(gameplay.NameMotion, inspect.CustomFunc(p.motionInspector)),
		e.Inspector(gameplay.NameHealth, inspect.CustomFunc(p.healthInspector)),
		e.Inspector(gameplay.NameStateMachine, inspect.CustomFunc(p.fsmInspector)),
	)
	if err := errors.Join(errs...); err != nil {
		return err
	}
	e.Overlay(p.sceneOverlay)
	e.GameOverlay(p.gameOverlay)
	e.Profiler("Gameplay", true, p.profile)
	if p.ed == nil {
		return nil
	}
	o := &p.ed.Overlay
	o.Set(CmdAreas, true)
	return e.Command(command.Desc{
		ID: CmdAreas, Name: "Gameplay Areas", Category: gameplay.Category, Order: 100,
		Checked: func() bool { return o.On(CmdAreas) },
		Run: func() error {
			o.Toggle(CmdAreas)
			return nil
		},
	})
}

func (p *plugin) areas() bool {
	return p.ed != nil && p.ed.Overlay.On(CmdAreas)
}
