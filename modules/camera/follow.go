package camera

import (
	"aego/core/mathx"
	"aego/render/view"
	"aego/scene"
)

func (s *Service) followOf(h scene.NodeHandle) *follow {
	for i := range s.follows {
		if s.follows[i].node == h {
			return &s.follows[i]
		}
	}
	return nil
}

func (s *Service) followFor(h scene.NodeHandle) *follow {
	if f := s.followOf(h); f != nil {
		return f
	}
	s.follows = append(s.follows, follow{node: h})
	return &s.follows[len(s.follows)-1]
}

func (s *Service) pruneFollows() {
	n := 0
	for _, f := range s.follows {
		if s.scene.IsAlive(f.node) && scene.Has[CameraFollow2D](s.scene, f.node) {
			s.follows[n] = f
			n++
		}
	}
	s.follows = s.follows[:n]
}

func (s *Service) Snap(h scene.NodeHandle) {
	if f := s.followOf(h); f != nil {
		f.snap = true
	}
}

func (s *Service) stepFollows(o *output, base view.View) {
	for _, h := range o.vcs {
		fc, ok := scene.Get[CameraFollow2D](s.scene, h)
		if !ok {
			continue
		}
		zoom := base.Camera.Zoom
		if vc, ok := scene.Get[VirtualCamera2D](s.scene, h); ok && vc.Zoom > 0 {
			zoom = vc.Zoom
		}
		s.stepFollow(s.followFor(h), fc, ViewSize(base, zoom))
	}
}

func (s *Service) stepFollow(f *follow, fc *CameraFollow2D, size mathx.Vec2) {
	th, ok := fc.Target.Resolve(s.scene)
	if !ok || !s.scene.IsAlive(th) {
		return
	}
	m := s.scene.RenderWorld(th)
	tp := mathx.Vec2{X: m[6], Y: m[7]}
	jump := fc.SnapDistance > 0 && tp.Sub(f.last).Len() > fc.SnapDistance
	if !f.ready || f.snap || th != f.target || jump {
		*f = follow{node: f.node, target: th, pos: tp, last: tp, ready: true}
		return
	}
	dt := s.dtGame
	if dt > 0 {
		f.vel = tp.Sub(f.last).Scale(1 / dt)
	}
	f.last = tp
	look, damping := fc.Lookahead, fc.Damping
	if s.reduced {
		look = mathx.Vec2{}
		damping = mathx.Vec2{X: max(damping.X, 8), Y: max(damping.Y, 8)}
	}
	want := mathx.Vec2{
		X: limit(f.vel.X*look.X, fc.MaxLookahead.X),
		Y: limit(f.vel.Y*look.Y, fc.MaxLookahead.Y),
	}
	f.look = f.look.Add(want.Sub(f.look).Scale(expK(fc.LookaheadSmoothing, dt)))
	f.pos = followStep(f.pos, tp.Add(f.look), size, fc.Deadzone, damping, dt)
}
