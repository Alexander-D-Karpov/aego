package dnd

import (
	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/draw"
)

type Payload struct {
	Kind  string
	Label string
	Data  any
}

type State struct {
	payload  Payload
	source   core.ID
	start    mathx.Vec2
	pos      mathx.Vec2
	pending  bool
	active   bool
	external bool
	Trace    func(event string, p Payload)
}

func (s *State) trace(event string) {
	if s.Trace != nil {
		s.Trace(event, s.payload)
	}
}

func (s *State) reset() {
	trace := s.Trace
	*s = State{}
	s.Trace = trace
}

func (s *State) Source(c *core.Context, id core.ID, p Payload) {
	if !s.active && c.Active() == id && c.MousePressed(input.MouseLeft) {
		s.payload = p
		s.source = id
		s.start = c.Mouse()
		s.pending = true
	}
	if !s.pending || s.source != id {
		return
	}
	if !c.MouseDown(input.MouseLeft) {
		s.pending = false
		return
	}
	if c.Mouse().Sub(s.start).Len() > 6 {
		s.pending = false
		s.active = true
		c.CaptureMouse(id)
		s.trace("drag started")
	}
}

func (s *State) External(p Payload, pos mathx.Vec2) {
	s.reset()
	s.payload = p
	s.active = true
	s.external = true
	s.pos = pos
}

func (s *State) Active() bool {
	return s.active
}

func (s *State) Pos(c *core.Context) mathx.Vec2 {
	if s.external {
		return s.pos
	}
	return c.Mouse()
}

func (s *State) Payload() (Payload, bool) {
	return s.payload, s.active
}

func (s *State) Is(kind string) bool {
	return s.active && s.payload.Kind == kind
}

func accepts(kinds []string, kind string) bool {
	if len(kinds) == 0 {
		return true
	}
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func (s *State) Hovering(c *core.Context, r mathx.Rect) bool {
	if !s.active {
		return false
	}
	pos := c.Mouse()
	if s.external {
		pos = s.pos
	}
	if !r.Contains(pos) {
		return false
	}
	if clip, ok := c.ClipRect(); ok && !clip.Contains(pos) {
		return false
	}
	return true
}

func (s *State) Target(c *core.Context, r mathx.Rect, kinds ...string) (Payload, bool) {
	if !s.Hovering(c, r) {
		return Payload{}, false
	}
	th := &c.Theme
	if !accepts(kinds, s.payload.Kind) {
		c.List().RectOutline(r, th.Error, 2)
		c.SetCursorShape(core.CursorNotAllowed)
		return Payload{}, false
	}
	c.Rect(r, th.DropTarget)
	c.List().RectOutline(r, th.Accent, 1)
	if s.external || c.MouseReleased(input.MouseLeft) {
		p := s.payload
		if !s.external {
			c.ReleaseMouse()
		}
		s.trace("dropped")
		s.reset()
		return p, true
	}
	return Payload{}, false
}

func (s *State) Frame(c *core.Context) {
	if !s.active {
		return
	}
	if s.external || !c.MouseDown(input.MouseLeft) {
		if !s.external {
			c.ReleaseMouse()
		}
		s.trace("drag cancelled, no target accepted it")
		trace := s.Trace
		s.reset()
		s.Trace = trace
		return
	}
	if s.payload.Label == "" {
		return
	}
	th := &c.Theme
	m := c.Mouse()
	size := c.TextSize(s.payload.Label)
	r := mathx.Rect{X: m.X + 14, Y: m.Y + 10, W: size.X + 2*th.Padding, H: th.RowHeight}
	l := c.List()
	l.PushLayer(draw.LayerTop)
	l.Rect(r, th.PanelAlt)
	l.RectOutline(r, th.Accent, 1)
	l.Text(s.payload.Label, mathx.Vec2{X: r.X + th.Padding, Y: r.Y + (r.H-c.LineHeight())/2}, th.Text)
	l.PopLayer()
}
