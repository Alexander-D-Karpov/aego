package debugdraw

import (
	"aego/core/mathx"
	"aego/scene"
)

const ServiceID = "debugdraw"

type LifeKind uint8

const (
	OneFrame LifeKind = iota
	Frames
	Seconds
	Persistent
)

type Domain uint8

const (
	Game Domain = iota
	Unscaled
)

type Lifetime struct {
	Kind   LifeKind
	N      float32
	Domain Domain
}

func ForFrames(n int) Lifetime {
	return Lifetime{Kind: Frames, N: float32(n)}
}

func ForSeconds(s float32) Lifetime {
	return Lifetime{Kind: Seconds, N: s}
}

func ForUnscaled(s float32) Lifetime {
	return Lifetime{Kind: Seconds, N: s, Domain: Unscaled}
}

func Forever() Lifetime {
	return Lifetime{Kind: Persistent}
}

type Space uint8

const (
	World Space = iota
	Screen
)

type Layer uint8

const (
	Scene Layer = iota
	Overlay
	BehindScene
)

type Fade uint8

const (
	FadeNone Fade = iota
	FadeAlpha
	FadeAlphaScale
)

type Style struct {
	Color     mathx.Color
	Thickness float32
	Life      Lifetime
	Channel   string
	Space     Space
	Layer     Layer
	Fade      Fade
	Owner     scene.NodeHandle
	Source    string
}

type Handle uint32

type Draw interface {
	Line(a, b mathx.Vec2, s *Style) Handle
	Arrow(from, to mathx.Vec2, s *Style) Handle
	Ray(origin, dir mathx.Vec2, s *Style) Handle
	Circle(center mathx.Vec2, r float32, s *Style) Handle
	Rect(r mathx.Rect, s *Style) Handle
	AABB(min, max mathx.Vec2, s *Style) Handle
	Polygon(pts []mathx.Vec2, closed bool, s *Style) Handle
	Cross(p mathx.Vec2, size float32, s *Style) Handle
	Point(p mathx.Vec2, s *Style) Handle
	Text(p mathx.Vec2, text string, s *Style) Handle
	Clear(h Handle)
	ClearChannel(prefix string)
	Wants(channel string) bool
	Hide(channel string, hidden bool)
	HideByDefault(prefix string)
	Enabled() bool
}
