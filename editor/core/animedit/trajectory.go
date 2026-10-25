package animedit

import (
	"aego/anim"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/scene"
	"aego/scene/propref"
	"aego/schema"
)

const (
	PathSamples = 64
	flashLife   = 0.8
)

type flash struct {
	t    float32
	life float32
}

// PathTrack is the position track of a node, bound in Preview or Record.
func (s *Service) PathTrack(node ids.NodeID) (int, bool) {
	if !s.applied {
		return -1, false
	}
	h := s.scene.Handle(node)
	for i := range s.binds {
		b := &s.binds[i]
		if b.node == h && b.pose && b.ref.Part() == propref.PartPosition && b.ref.Lane() < 0 && s.clip.Tracks[b.track].Kind == schema.KindVec2 {
			return b.track, true
		}
	}
	return -1, false
}

// Path samples a position track over the clip into world points, keys last.
func (s *Service) Path(node ids.NodeID, track int, pts, keys []mathx.Vec2) ([]mathx.Vec2, []mathx.Vec2) {
	pts, keys = pts[:0], keys[:0]
	h := s.scene.Handle(node)
	if h.IsZero() || track < 0 {
		return pts, keys
	}
	w := s.parentWorld(h)
	var smp anim.Sampler
	smp.Reset(s.clip)
	for i := 0; i <= PathSamples; i++ {
		t := s.clip.Length * float32(i) / PathSamples
		v := smp.Track(s.clip, track, t)
		pts = append(pts, w.Apply(mathx.Vec2{X: v.L[0], Y: v.L[1]}))
	}
	for _, k := range s.clip.Tracks[track].Keys {
		keys = append(keys, w.Apply(mathx.Vec2{X: k.V.L[0], Y: k.V.L[1]}))
	}
	return pts, keys
}

// Flashes are the world points of preview events that fired in the last moment, with their fade.
func (s *Service) Flashes(node ids.NodeID, track int, dst []mathx.Vec2, fade []float32) ([]mathx.Vec2, []float32) {
	dst, fade = dst[:0], fade[:0]
	h := s.scene.Handle(node)
	if h.IsZero() || track < 0 {
		return dst, fade
	}
	w := s.parentWorld(h)
	for _, f := range s.flash {
		v := s.samp.Track(s.clip, track, f.t)
		dst = append(dst, w.Apply(mathx.Vec2{X: v.L[0], Y: v.L[1]}))
		fade = append(fade, f.life/flashLife)
	}
	return dst, fade
}

func (s *Service) parentWorld(h scene.NodeHandle) mathx.Mat3 {
	if p := s.scene.Parent(h); !p.IsZero() {
		return s.scene.World(p)
	}
	return mathx.Mat3Identity()
}

// MovePathKey sets key i of a position track to a world point.
func (s *Service) MovePathKey(node ids.NodeID, track, i int, world mathx.Vec2) error {
	if s.clip == nil || track < 0 || i >= len(s.clip.Tracks[track].Keys) {
		return errNoClip
	}
	h := s.scene.Handle(node)
	inv, ok := s.parentWorld(h).Inverse()
	if !ok {
		return nil
	}
	l := inv.Apply(world)
	tr := &s.clip.Tracks[track]
	k := tr.Keys[i]
	op := s.setOpKind(tr, timeText(k.T), anim.Lanes([4]float32{l.X, l.Y}))
	return s.EditClip(i18n.Tr("editor.history.move_path_key", "Move Path Key"), op)
}

// HoldPose shows the scene transform instead of the sampled pose while a gizmo drags in Record.
func (s *Service) HoldPose(on bool) {
	if s.hold == on {
		return
	}
	s.hold = on
	if on {
		for h := range s.posed {
			s.scene.DropPresentationLocal(h)
			delete(s.posed, h)
		}
		return
	}
	s.sample()
}
