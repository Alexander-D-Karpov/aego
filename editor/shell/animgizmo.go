package shell

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/animedit"
	"aego/editor/core/gizmo"
	"aego/modules/animation"
)

const pathPick = 8

// pathGizmo draws the position track of the animated node and drags its keys (D337).
type pathGizmo struct {
	a     *App
	pts   []mathx.Vec2
	keys  []mathx.Vec2
	fl    []mathx.Vec2
	fade  []float32
	hit   int
	drag  int
	track int
	node  ids.NodeID
}

func (a *App) registerPathGizmo() {
	g := &pathGizmo{a: a, hit: -1, drag: -1}
	sr := a.opt.Types.Schema()
	for _, name := range []string{animation.NamePlayer, animation.NameAnimator} {
		if td, ok := sr.ByName(name); ok {
			_ = a.gizmos.Register(td.ID, g)
		}
	}
}

func (a *App) animDragging(on bool) {
	if d := a.ctx.Doc(); d != nil && a.ctx.Anim != nil {
		if s := a.ctx.Anim.For(d); s.Mode() == animedit.Record {
			s.HoldPose(on)
		}
	}
}

func (g *pathGizmo) svc() *animedit.Service {
	d := g.a.ctx.Doc()
	if d == nil || g.a.ctx.Anim == nil {
		return nil
	}
	return g.a.ctx.Anim.For(d)
}

func (g *pathGizmo) load(node ids.NodeID) (*animedit.Service, int, bool) {
	s := g.svc()
	if s == nil || s.Mode() == animedit.Normal {
		return nil, -1, false
	}
	track, ok := s.PathTrack(node)
	if !ok {
		return nil, -1, false
	}
	g.pts, g.keys = s.Path(node, track, g.pts, g.keys)
	return s, track, true
}

func (g *pathGizmo) Draw(c *gizmo.DrawCtx) {
	s, track, ok := g.load(c.Node)
	if !ok {
		return
	}
	col := c.Colors.Rotate
	line := col
	line.A *= 0.6
	for i := 1; i < len(g.pts); i++ {
		c.Line(g.pts[i-1], g.pts[i], line)
	}
	r := c.PxToWorld(4)
	for i, k := range g.keys {
		kc := col
		if i == g.drag || (g.drag < 0 && i == g.hit && c.Hot != gizmo.PartNone) {
			kc = c.Colors.Hot
		}
		c.Debug.Circle(k, r, 10, kc)
	}
	g.fl, g.fade = s.Flashes(c.Node, track, g.fl, g.fade)
	for i, p := range g.fl {
		fc := c.Colors.Center
		fc.A *= g.fade[i]
		c.Debug.Circle(p, c.PxToWorld(6+6*(1-g.fade[i])), 16, fc)
	}
}

func (g *pathGizmo) HitTest(c *gizmo.HitCtx) (gizmo.Hit, bool) {
	g.hit = -1
	if _, _, ok := g.load(c.Node); !ok {
		return gizmo.Hit{}, false
	}
	best := float32(pathPick)
	for i, k := range g.keys {
		if d := c.Space.ToPixels(k).Sub(c.MousePx).Len(); d <= best {
			best, g.hit = d, i
		}
	}
	if g.hit < 0 {
		return gizmo.Hit{}, false
	}
	return gizmo.Hit{Part: gizmo.PartXY, Dist: best}, true
}

func (g *pathGizmo) Begin(e *gizmo.EditCtx, _ gizmo.Hit) {
	_, track, ok := g.load(e.Node)
	if !ok || g.hit < 0 {
		return
	}
	g.drag, g.track, g.node = g.hit, track, e.Node
}

func (g *pathGizmo) Update(e *gizmo.EditCtx) {
	if g.drag < 0 {
		return
	}
	if s := g.svc(); s != nil {
		e.Err = s.MovePathKey(g.node, g.track, g.drag, e.Mouse)
	}
}

func (g *pathGizmo) End(*gizmo.EditCtx) {
	g.drag = -1
}
