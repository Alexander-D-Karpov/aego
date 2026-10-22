package world

import (
	"errors"
	"fmt"
	"time"

	"aego/core/handle"
)

var (
	ErrInvalid = errors.New("physics: invalid definition")
	ErrHandle  = errors.New("physics: stale or foreign handle")
)

func bad(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalid}, args...)...)
}

type (
	BodyHandle  = handle.Handle[body]
	ShapeHandle = handle.Handle[shape]
	JointHandle = handle.Handle[joint]
)

type BodyType uint8

const (
	Static BodyType = iota
	Kinematic
	Dynamic
)

func (t BodyType) String() string {
	switch t {
	case Static:
		return "static"
	case Kinematic:
		return "kinematic"
	case Dynamic:
		return "dynamic"
	}
	return "unknown"
}

type MassMode uint8

const (
	MassAuto MassMode = iota
	MassOverride
)

type Transform struct {
	Position Vec2
	Angle    float32
}

func (t Transform) xf() xform {
	return xform{p: t.Position, q: makeRot(t.Angle)}
}

type BodyDef struct {
	Type            BodyType
	Position        Vec2
	Angle           float32
	LinearVelocity  Vec2
	AngularVelocity float32
	MassMode        MassMode
	Mass            float32
	GravityScale    float32
	LinearDamping   float32
	AngularDamping  float32
	FixedRotation   bool
	Bullet          bool
	AllowSleep      bool
	Layer           uint8
	Mask            uint32
	UserData        uint64
}

func DefaultBodyDef() BodyDef {
	return BodyDef{Type: Dynamic, GravityScale: 1, AllowSleep: true, Mask: ^uint32(0)}
}

type ShapeDef struct {
	Geometry    Geometry
	Local       Transform
	IsTrigger   bool
	Friction    float32
	Restitution float32
	Density     float32
	OneSided    bool
	UserData    uint64
}

func DefaultShapeDef(g Geometry) ShapeDef {
	return ShapeDef{Geometry: g, Friction: 0.6, Density: 1}
}

type EventKind uint8

const (
	BeginContact EventKind = iota
	EndContact
	BeginTrigger
	EndTrigger
)

func (k EventKind) String() string {
	switch k {
	case BeginContact:
		return "BeginContact"
	case EndContact:
		return "EndContact"
	case BeginTrigger:
		return "BeginTrigger"
	case EndTrigger:
		return "EndTrigger"
	}
	return "Unknown"
}

type ContactEvent struct {
	Kind             EventKind
	ShapeA, ShapeB   ShapeHandle
	BodyA, BodyB     BodyHandle
	Point, Normal    Vec2
	NormalImpulse    float32
	RelativeVelocity Vec2
}

type Events struct {
	Contacts []ContactEvent
}

type QueryFilter struct {
	LayerMask       uint32
	IncludeTriggers bool
	SkipDynamic     bool
	ExcludeBody     BodyHandle
}

func DefaultQueryFilter() QueryFilter {
	return QueryFilter{LayerMask: ^uint32(0)}
}

type RayHit struct {
	Point    Vec2
	Normal   Vec2
	Distance float32
	Fraction float32
	Shape    ShapeHandle
	Body     BodyHandle
}

type StageTimes struct {
	Sync      time.Duration
	Broad     time.Duration
	Narrow    time.Duration
	Solve     time.Duration
	Integrate time.Duration
	Events    time.Duration
}

type Stats struct {
	Bodies               int
	Active               int
	Sleeping             int
	Shapes               int
	BroadphaseCandidates int
	TouchingPairs        int
	Manifolds            int
	ContactPoints        int
	Joints               int
	Islands              int
	Steps                uint64
	Time                 StageTimes
}
