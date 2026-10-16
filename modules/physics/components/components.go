package components

import (
	"aego/core/mathx"
	"aego/scene"
	"aego/schema"
)

type BodyType uint8

const (
	Static BodyType = iota
	Kinematic
	Dynamic
)

type MassMode uint8

const (
	MassAuto MassMode = iota
	MassOverride
)

const (
	NameBody     = "physics.Body2D"
	NameBox      = "physics.BoxCollider2D"
	NameCircle   = "physics.CircleCollider2D"
	NameCapsule  = "physics.CapsuleCollider2D"
	NamePolygon  = "physics.PolygonCollider2D"
	NameEdge     = "physics.EdgeCollider2D"
	NameDistance = "physics.DistanceJoint2D"
	NameRevolute = "physics.RevoluteJoint2D"

	MaxPolygonPoints = 8
)

type Kind uint8

const (
	KindBox Kind = iota
	KindCircle
	KindCapsule
	KindPolygon
	KindEdge
	KindCount
)

var Names = [KindCount]string{NameBox, NameCircle, NameCapsule, NamePolygon, NameEdge}

var (
	TypeBody      = schema.HashName(NameBody)
	TypeDistance  = schema.HashName(NameDistance)
	TypeRevolute  = schema.HashName(NameRevolute)
	colliderTypes = [KindCount]schema.TypeID{
		schema.HashName(NameBox),
		schema.HashName(NameCircle),
		schema.HashName(NameCapsule),
		schema.HashName(NamePolygon),
		schema.HashName(NameEdge),
	}
)

func ColliderType(k Kind) schema.TypeID {
	return colliderTypes[k]
}

func Is(t schema.TypeID) bool {
	if t == TypeBody || t == TypeDistance || t == TypeRevolute {
		return true
	}
	for _, c := range colliderTypes {
		if c == t {
			return true
		}
	}
	return false
}

type Body2D struct {
	Type            BodyType   `aego:"type"`
	MassMode        MassMode   `aego:"massMode"`
	Mass            float32    `aego:"mass,min=0,hidden"`
	GravityScale    float32    `aego:"gravityScale"`
	LinearDamping   float32    `aego:"linearDamping,min=0"`
	AngularDamping  float32    `aego:"angularDamping,min=0"`
	FixedRotation   bool       `aego:"fixedRotation"`
	Bullet          bool       `aego:"bullet"`
	AllowSleep      bool       `aego:"allowSleep"`
	Interpolate     bool       `aego:"interpolate"`
	Layer           int32      `aego:"layer,min=0,max=31"`
	Mask            int64      `aego:"mask"`
	Velocity        mathx.Vec2 `aego:"velocity,transient,readonly,category=Runtime"`
	AngularVelocity float32    `aego:"angularVelocity,transient,readonly,category=Runtime"`
	Awake           bool       `aego:"awake,transient,readonly,category=Runtime"`
}

type ColliderCommon struct {
	Offset      mathx.Vec2 `aego:"offset"`
	IsTrigger   bool       `aego:"isTrigger"`
	Friction    float32    `aego:"friction,min=0"`
	Restitution float32    `aego:"restitution,min=0"`
	Density     float32    `aego:"density,min=0"`
}

type BoxCollider2D struct {
	ColliderCommon
	Size mathx.Vec2 `aego:"size"`
}

type CircleCollider2D struct {
	ColliderCommon
	Radius float32 `aego:"radius,min=0"`
}

type CapsuleCollider2D struct {
	ColliderCommon
	Length float32 `aego:"length,min=0"`
	Radius float32 `aego:"radius,min=0"`
}

type PolygonCollider2D struct {
	ColliderCommon
	Points []mathx.Vec2 `aego:"points"`
}

type EdgeCollider2D struct {
	ColliderCommon
	Points   []mathx.Vec2 `aego:"points"`
	Loop     bool         `aego:"loop"`
	OneSided bool         `aego:"oneSided"`
}

type DistanceJoint2D struct {
	Target           scene.NodeRef `aego:"target"`
	AnchorA          mathx.Vec2    `aego:"anchorA"`
	AnchorB          mathx.Vec2    `aego:"anchorB"`
	Length           float32       `aego:"length,min=0"`
	MinLength        float32       `aego:"minLength,min=0"`
	MaxLength        float32       `aego:"maxLength,min=0"`
	Stiffness        float32       `aego:"stiffness,min=0"`
	Damping          float32       `aego:"damping,min=0"`
	CollideConnected bool          `aego:"collideConnected"`
}

type RevoluteJoint2D struct {
	Target           scene.NodeRef `aego:"target"`
	Anchor           mathx.Vec2    `aego:"anchor"`
	EnableLimit      bool          `aego:"enableLimit"`
	Lower            float32       `aego:"lower"`
	Upper            float32       `aego:"upper"`
	EnableMotor      bool          `aego:"enableMotor"`
	Speed            float32       `aego:"speed"`
	MaxTorque        float32       `aego:"maxTorque,min=0"`
	CollideConnected bool          `aego:"collideConnected"`
}

func DefaultBody2D() Body2D {
	return Body2D{Type: Dynamic, GravityScale: 1, AllowSleep: true, Mask: -1}
}

func DefaultCommon() ColliderCommon {
	return ColliderCommon{Friction: 0.6, Density: 1}
}

func DefaultBox() BoxCollider2D {
	return BoxCollider2D{ColliderCommon: DefaultCommon(), Size: mathx.Vec2{X: 1, Y: 1}}
}

func DefaultCircle() CircleCollider2D {
	return CircleCollider2D{ColliderCommon: DefaultCommon(), Radius: 0.5}
}

func DefaultCapsule() CapsuleCollider2D {
	return CapsuleCollider2D{ColliderCommon: DefaultCommon(), Length: 1, Radius: 0.25}
}

func DefaultPolygon() PolygonCollider2D {
	return PolygonCollider2D{ColliderCommon: DefaultCommon(), Points: []mathx.Vec2{
		{X: -0.5, Y: -0.5}, {X: 0.5, Y: -0.5}, {X: 0.5, Y: 0.5}, {X: -0.5, Y: 0.5},
	}}
}

func DefaultEdge() EdgeCollider2D {
	return EdgeCollider2D{ColliderCommon: DefaultCommon(), Points: []mathx.Vec2{{X: -2}, {X: 2}}}
}

func DefaultDistance() DistanceJoint2D {
	return DistanceJoint2D{}
}

func DefaultRevolute() RevoluteJoint2D {
	return RevoluteJoint2D{}
}

type Collider struct {
	Kind     Kind
	Common   ColliderCommon
	Size     mathx.Vec2
	Radius   float32
	Length   float32
	Points   []mathx.Vec2
	Loop     bool
	OneSided bool
}

func HasCollider(s *scene.Scene, h scene.NodeHandle, k Kind) bool {
	switch k {
	case KindBox:
		return scene.Has[BoxCollider2D](s, h)
	case KindCircle:
		return scene.Has[CircleCollider2D](s, h)
	case KindCapsule:
		return scene.Has[CapsuleCollider2D](s, h)
	case KindPolygon:
		return scene.Has[PolygonCollider2D](s, h)
	case KindEdge:
		return scene.Has[EdgeCollider2D](s, h)
	}
	return false
}

func Read(s *scene.Scene, h scene.NodeHandle, k Kind, dst *Collider) bool {
	*dst = Collider{Kind: k}
	switch k {
	case KindBox:
		c, ok := scene.Get[BoxCollider2D](s, h)
		if !ok {
			return false
		}
		dst.Common, dst.Size = c.ColliderCommon, c.Size
	case KindCircle:
		c, ok := scene.Get[CircleCollider2D](s, h)
		if !ok {
			return false
		}
		dst.Common, dst.Radius = c.ColliderCommon, c.Radius
	case KindCapsule:
		c, ok := scene.Get[CapsuleCollider2D](s, h)
		if !ok {
			return false
		}
		dst.Common, dst.Length, dst.Radius = c.ColliderCommon, c.Length, c.Radius
	case KindPolygon:
		c, ok := scene.Get[PolygonCollider2D](s, h)
		if !ok {
			return false
		}
		dst.Common, dst.Points = c.ColliderCommon, c.Points
	case KindEdge:
		c, ok := scene.Get[EdgeCollider2D](s, h)
		if !ok {
			return false
		}
		dst.Common, dst.Points, dst.Loop, dst.OneSided = c.ColliderCommon, c.Points, c.Loop, c.OneSided
	default:
		return false
	}
	return true
}
