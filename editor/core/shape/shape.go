package shape

import (
	"aego/asset"
	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/engine"
	"aego/render/debug"
	"aego/render/view"
	"aego/scene"
	"aego/schema"
)

type Kind uint8

const (
	None Kind = iota
	Rect
	Box
)

type Shape struct {
	Kind  Kind
	Local mathx.Rect
	Box   mathx.AABB3
	Order int64
}

type Func func(s *scene.Scene, res asset.Resolver, h scene.NodeHandle) (Shape, bool)

type OverlayCtx struct {
	Scene  *scene.Scene
	World  *engine.World
	Debug  *debug.Draw
	View   view.View
	Mode3D bool
	Game   bool
	Window mathx.Vec2
	Px     float32
	Aspect float32
	Color  mathx.Color
}

type Overlay func(*OverlayCtx)

type entry struct {
	owner string
	t     schema.TypeID
	fn    Func
}

type overlay struct {
	owner string
	fn    Overlay
	game  bool
}

type Registry struct {
	entries  []entry
	overlays []overlay
}

func NewRegistry() *Registry {
	return &Registry{}
}

func (r *Registry) Register(owner string, t schema.TypeID, fn Func) error {
	if fn == nil {
		return errs.New(errs.EditorPanel, "bounds function is nil", log.F("module", owner))
	}
	for _, e := range r.entries {
		if e.t == t {
			return errs.New(errs.EditorPanel, "bounds are already registered for this component", log.F("module", owner))
		}
	}
	r.entries = append(r.entries, entry{owner: owner, t: t, fn: fn})
	return nil
}

func (r *Registry) AddOverlay(owner string, fn Overlay) {
	r.AddOverlayIn(owner, fn, false)
}

func (r *Registry) AddOverlayIn(owner string, fn Overlay, game bool) {
	if fn != nil {
		r.overlays = append(r.overlays, overlay{owner: owner, fn: fn, game: game})
	}
}

func (r *Registry) Remove(owner string) {
	entries := r.entries[:0]
	for _, e := range r.entries {
		if e.owner != owner {
			entries = append(entries, e)
		}
	}
	r.entries = entries
	overlays := r.overlays[:0]
	for _, o := range r.overlays {
		if o.owner != owner {
			overlays = append(overlays, o)
		}
	}
	r.overlays = overlays
}

type Set struct {
	Reg *Registry
	Res asset.Resolver
}

func (s Set) Of(sc *scene.Scene, h scene.NodeHandle) (Shape, bool) {
	if s.Reg == nil || sc == nil || h.IsZero() || !sc.IsAlive(h) {
		return Shape{}, false
	}
	var best Shape
	found := false
	for _, e := range s.Reg.entries {
		if _, ok := sc.GetByType(h, e.t); !ok {
			continue
		}
		if sh, ok := e.fn(sc, s.Res, h); ok && (!found || sh.Order > best.Order) {
			best, found = sh, true
		}
	}
	return best, found
}

func (s Set) Overlays(c *OverlayCtx) {
	if s.Reg == nil {
		return
	}
	for _, o := range s.Reg.overlays {
		if c.Game && !o.game {
			continue
		}
		o.fn(c)
	}
}
