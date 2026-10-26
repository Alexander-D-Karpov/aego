package gameplay

import (
	"errors"

	"aego/core/mathx"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
)

const (
	NameMotion = "gameplay.Motion2D"
	bodyName   = "physics.Body2D"
	layersList = "layers"

	CodeNotKinematic = "GMP001"
	CodeNoJump       = "GMP002"
)

type Mode uint8

const (
	Platformer Mode = iota
	TopDown
)

type LeaveMode uint8

const (
	LeaveKeep LeaveMode = iota
	LeaveDrop
	LeaveAdd
)

const (
	evLanded uint8 = 1 << iota
	evLeft
	evJumped
	evWall
	evCeiling
	evClimbed
)

type motionState struct {
	move       mathx.Vec2
	jumpReq    bool
	jumpRel    bool
	jumping    bool
	dashVel    mathx.Vec2
	dashLeft   float32
	addVel     mathx.Vec2
	stop       bool
	teleport   bool
	teleportTo mathx.Vec2
	keepVel    bool
	coyote     float32
	buffer     float32
	grounded   bool
	floorN     mathx.Vec2
	ground     scene.NodeHandle
	platVel    mathx.Vec2
	events     uint8
	landSpeed  float32
	wallN      mathx.Vec2
	disp       mathx.Vec2
	climbFrom  mathx.Vec2
	climbRise  mathx.Vec2
	climbFwd   mathx.Vec2
	climbLeft  float32
	climbFloor scene.NodeHandle
	warned     bool
}

type Motion2D struct {
	Mode            Mode          `aego:"mode"`
	UpDirection     mathx.Vec2    `aego:"upDirection"`
	MaxSpeed        float32       `aego:"maxSpeed,min=0"`
	Accel           float32       `aego:"accel,min=0"`
	Decel           float32       `aego:"decel,min=0"`
	AirControl      float32       `aego:"airControl,min=0,max=1"`
	GravityScale    float32       `aego:"gravityScale"`
	MaxFallSpeed    float32       `aego:"maxFallSpeed,min=0"`
	JumpHeight      float32       `aego:"jumpHeight,min=0"`
	JumpSpeed       float32       `aego:"jumpSpeed,min=0"`
	JumpCut         float32       `aego:"jumpCutMultiplier,min=0,max=1"`
	CoyoteTime      float32       `aego:"coyoteTime,min=0"`
	JumpBuffer      float32       `aego:"jumpBuffer,min=0"`
	MaxSlope        float32       `aego:"maxSlope,min=0,max=89"`
	StepHeight      float32       `aego:"stepHeight,min=0"`
	SnapToGround    float32       `aego:"snapToGround,min=0"`
	SafeMargin      float32       `aego:"safeMargin,min=0.001"`
	MaxSlides       int32         `aego:"maxSlides,min=1,max=8"`
	CollisionMask   int64         `aego:"collisionMask"`
	PushBodies      bool          `aego:"pushBodies"`
	MovingPlatforms bool          `aego:"movingPlatforms"`
	PlatformOnLeave LeaveMode     `aego:"platformOnLeave"`
	FaceMovement    bool          `aego:"faceMovement"`
	DashGravity     bool          `aego:"dashGravity"`
	LedgeClimb      float32       `aego:"ledgeClimb,min=0"`
	ClimbTime       float32       `aego:"climbTime,min=0"`
	Velocity        mathx.Vec2    `aego:"velocity,transient,readonly,category=Runtime"`
	Grounded        bool          `aego:"grounded,transient,readonly,category=Runtime"`
	OnWall          bool          `aego:"onWall,transient,readonly,category=Runtime"`
	OnCeiling       bool          `aego:"onCeiling,transient,readonly,category=Runtime"`
	WallNormal      mathx.Vec2    `aego:"wallNormal,transient,readonly,category=Runtime"`
	FloorNormal     mathx.Vec2    `aego:"floorNormal,transient,readonly,category=Runtime"`
	FloorAngle      float32       `aego:"floorAngle,transient,readonly,category=Runtime"`
	GroundBody      scene.NodeRef `aego:"groundBody,transient,readonly,category=Runtime"`
	Climbing        bool          `aego:"climbing,transient,readonly,category=Runtime"`

	st motionState
}

func DefaultMotion() Motion2D {
	return Motion2D{
		UpDirection: mathx.Vec2{Y: -1}, MaxSpeed: 6, Accel: 60, Decel: 50, AirControl: 0.6,
		GravityScale: 1, MaxFallSpeed: 20, JumpHeight: 2.2, JumpCut: 0.5, CoyoteTime: 0.1, JumpBuffer: 0.1,
		MaxSlope: 45, StepHeight: 0.2, SnapToGround: 0.2, SafeMargin: 0.02, MaxSlides: 4, CollisionMask: -1, ClimbTime: 0.3,
		PushBodies: true, MovingPlatforms: true,
	}
}

func (m *Motion2D) Move(x float32) {
	m.st.move = mathx.Vec2{X: mathx.Clamp(x, -1, 1)}
}

func (m *Motion2D) MoveDir(v mathx.Vec2) {
	m.st.move = v
}

func (m *Motion2D) Jump() {
	m.st.jumpReq = true
}

func (m *Motion2D) JumpReleased() {
	m.st.jumpRel = true
}

func (m *Motion2D) Dash(v mathx.Vec2, seconds float32) {
	m.st.dashVel, m.st.dashLeft = v, max(seconds, 0)
}

func (m *Motion2D) AddVelocity(v mathx.Vec2) {
	m.st.addVel = m.st.addVel.Add(v)
}

func (m *Motion2D) AddDisplacement(d mathx.Vec2) {
	m.st.disp = m.st.disp.Add(d)
}

func (m *Motion2D) Stop() {
	m.st.stop = true
}

func (m *Motion2D) Teleport(p mathx.Vec2) {
	m.st.teleport, m.st.teleportTo, m.st.keepVel = true, p, false
}

// teleportKeep moves like Teleport but keeps the velocity.
func (m *Motion2D) teleportKeep(p mathx.Vec2) {
	m.st.teleport, m.st.teleportTo, m.st.keepVel = true, p, true
}

const kinematic = 1

type signals struct {
	landed    module.Signal1[float32]
	left      module.Signal0
	jumped    module.Signal0
	wall      module.Signal1[mathx.Vec2]
	ceiling   module.Signal0
	climbed   module.Signal0
	damaged   module.Signal2[int64, scene.NodeHandle]
	healed    module.Signal1[int64]
	died      module.Signal1[scene.NodeHandle]
	revived   module.Signal0
	hit       module.Signal2[scene.NodeHandle, int64]
	reached   module.Signal1[scene.NodeHandle]
	respawned module.Signal0
	collected module.Signal1[scene.NodeHandle]
	changed   module.Signal2[int64, int64]
}

func bodyType(s *scene.Scene) (*schema.TypeDescriptor, schema.FieldID, bool) {
	td, ok := s.Registry().ByName(bodyName)
	if !ok {
		return nil, 0, false
	}
	fd, ok := td.Field("type")
	if !ok {
		return nil, 0, false
	}
	return td, fd.ID, true
}

func validateMotion(ctx module.ValidateContext, m *Motion2D) []problems.Diagnostic {
	var out []problems.Diagnostic
	if td, f, ok := bodyType(ctx.Scene); ok {
		if v, err := ctx.Scene.GetProperty(ctx.Node, td.ID, f); err == nil && v.I != kinematic {
			out = append(out, problems.Diagnostic{Code: CodeNotKinematic, Severity: problems.Error, BlocksPlay: true,
				Message: "Motion 2D moves its body itself: set the Type of Rigid Body 2D to kinematic"})
		}
	}
	if m.Mode == Platformer && m.GravityScale == 0 && m.JumpSpeed == 0 && m.JumpHeight > 0 {
		out = append(out, problems.Diagnostic{Code: CodeNoJump, Severity: problems.Error,
			Message: "Motion 2D: with Gravity Scale 0 the jump speed cannot come from Jump Height, set Jump Speed"})
	}
	return out
}

func kinematicOnAdd(ctx module.AddContext, _ *Motion2D) {
	if td, f, ok := bodyType(ctx.Scene); ok {
		_ = ctx.Scene.SetProperty(ctx.Node, td.ID, f, schema.Enum(kinematic))
	}
}

func registerMotion(r *module.Registry, sig *signals) error {
	if err := errors.Join(
		module.Enum[Mode](r, "gameplay.MotionMode",
			schema.EnumValue{Name: "platformer", Value: int64(Platformer)},
			schema.EnumValue{Name: "topDown", Value: int64(TopDown)}),
		module.Enum[LeaveMode](r, "gameplay.PlatformLeave",
			schema.EnumValue{Name: "keep", Value: int64(LeaveKeep)},
			schema.EnumValue{Name: "drop", Value: int64(LeaveDrop)},
			schema.EnumValue{Name: "add", Value: int64(LeaveAdd)}),
	); err != nil {
		return err
	}
	b := module.Component[Motion2D](r, schema.Name(NameMotion), schema.Space(schema.Space2D),
		schema.Doc("Moves a kinematic Body2D by casting its colliders: walking, jumping, slopes, steps and moving platforms, or free top-down movement. Call Move and Jump from a script; the controller acts on the next fixed step."),
		schema.FieldDoc("mode", "platformer walks on floors under gravity; topDown moves freely in the plane."),
		schema.FieldDoc("upDirection", "Up for floors, jumps and gravity. (0, -1) is up on screen."),
		schema.FieldDoc("maxSpeed", "Top speed in units per second."),
		schema.FieldDoc("accel", "How fast the speed grows towards the target, units per second squared."),
		schema.FieldDoc("decel", "How fast the speed falls without input, in units per second squared."),
		schema.FieldDoc("airControl", "Share of Accel and Decel available in the air: 0 cannot steer, 1 steers as on the ground."),
		schema.FieldDoc("gravityScale", "Multiplies the physics gravity; 0 turns gravity off for the character."),
		schema.FieldDoc("maxFallSpeed", "Fastest fall in units per second. 0 is unlimited."),
		schema.FieldDoc("jumpHeight", "Height of a full jump in units; sets the jump speed unless Jump Speed is set."),
		schema.FieldDoc("jumpSpeed", "Upward speed of a jump. 0 derives it from Jump Height and gravity."),
		schema.FieldDoc("jumpCutMultiplier", "Upward speed is multiplied by this when the jump button is released early."),
		schema.FieldDoc("coyoteTime", "Seconds after leaving a ledge during which a jump still works."),
		schema.FieldDoc("jumpBuffer", "Seconds a jump pressed in the air is remembered until landing."),
		schema.FieldDoc("maxSlope", "Steepest floor in degrees; steeper surfaces are walls."),
		schema.FieldDoc("stepHeight", "Steps up to this height, in units, are climbed without jumping. 0 never steps up."),
		schema.FieldDoc("snapToGround", "Keeps the character on slopes and steps down by up to this distance, in units. 0 never snaps."),
		schema.FieldDoc("safeMargin", "Gap in units kept to surfaces so casts never start inside them."),
		schema.FieldDoc("maxSlides", "How many surfaces one step may slide along."),
		schema.FieldDoc("collisionMask", "Layers the character collides with."),
		schema.FieldDoc("pushBodies", "Moves through dynamic bodies and pushes them instead of stopping at them."),
		schema.FieldDoc("movingPlatforms", "Carries the character with the body it stands on."),
		schema.FieldDoc("platformOnLeave", "keep keeps the platform's velocity after leaving it, drop loses it, add keeps only its horizontal part."),
		schema.FieldDoc("faceMovement", "topDown: turns the node towards the movement."),
		schema.FieldDoc("dashGravity", "Applies gravity during a dash."),
		schema.FieldDoc("ledgeClimb", "platformer: in the air, moving into a wall whose top is at most this far above the feet climbs onto it. 0 never climbs."),
		schema.FieldDoc("climbTime", "Seconds a ledge climb takes: rising, then stepping onto the ledge."),
		schema.FieldDoc("climbing", "Whether the character is climbing onto a ledge."),
		schema.FieldDoc("velocity", "Velocity in units per second."),
		schema.FieldDoc("grounded", "Whether the character stands on a floor."),
		schema.FieldDoc("onWall", "Whether the character touched a wall in the last step."),
		schema.FieldDoc("onCeiling", "Whether the character touched a ceiling in the last step."),
		schema.FieldDoc("wallNormal", "Normal of the last wall touched."),
		schema.FieldDoc("floorNormal", "Normal of the floor under the character."),
		schema.FieldDoc("floorAngle", "Slope of the floor in degrees."),
		schema.FieldDoc("groundBody", "Body the character stands on."),
		schema.Runtime("velocity", "grounded", "onWall", "onCeiling", "wallNormal", "floorNormal", "floorAngle", "groundBody", "climbing"),
		schema.Bitset("collisionMask", needsPhysics, layersList),
	).Requires(bodyName).Suggests(capsuleName, boxName).Category(Category).Display("Motion 2D").Icon("gameplay.motion").NodeIcon(75).Default(DefaultMotion).Validate(validateMotion).OnAdd(kinematicOnAdd)

	b.Proc1("Move", func(_ *bind.Ctx, m *Motion2D, x float32) { m.Move(x) },
		bind.Doc("Move sets the horizontal input, -1..1; it stays until the next Move."), bind.Params("x"))
	b.Proc1("MoveDir", func(_ *bind.Ctx, m *Motion2D, v mathx.Vec2) { m.MoveDir(v) },
		bind.Doc("MoveDir sets the top-down input; a longer vector is cut to length 1."), bind.Params("dir"))
	b.Proc0("Jump", func(_ *bind.Ctx, m *Motion2D) { m.Jump() },
		bind.Doc("Jump jumps on the next step if grounded, within coyote time or later within the jump buffer."))
	b.Proc0("JumpReleased", func(_ *bind.Ctx, m *Motion2D) { m.JumpReleased() },
		bind.Doc("JumpReleased cuts a rising jump by Jump Cut Multiplier."))
	b.Proc2("Dash", func(_ *bind.Ctx, m *Motion2D, v mathx.Vec2, s float32) { m.Dash(v, s) },
		bind.Doc("Dash moves at a fixed velocity for some seconds."), bind.Params("velocity", "seconds"))
	b.Proc1("AddVelocity", func(_ *bind.Ctx, m *Motion2D, v mathx.Vec2) { m.AddVelocity(v) },
		bind.Doc("AddVelocity adds a push once, for knockback."), bind.Params("velocity"))
	b.Proc1("AddDisplacement", func(_ *bind.Ctx, m *Motion2D, d mathx.Vec2) { m.AddDisplacement(d) },
		bind.Doc("AddDisplacement moves by an offset in world units on the next step, through walls and slopes like a move, without changing the velocity; root motion of an Animator uses it."),
		bind.Params("offset"))
	b.Proc0("Stop", func(_ *bind.Ctx, m *Motion2D) { m.Stop() },
		bind.Doc("Stop zeroes the velocity and ends a dash."))
	b.Proc1("Teleport", func(_ *bind.Ctx, m *Motion2D, p mathx.Vec2) { m.Teleport(p) },
		bind.Doc("Teleport moves to a position on the next step and resets velocity and state."), bind.Params("position"))

	sig.landed = b.Signal1[float32]("Landed", "Landed fires when the character touches a floor, with the falling speed.", "speed")
	sig.left = b.Signal0("LeftGround", "LeftGround fires when the character walks off a floor without jumping.")
	sig.jumped = b.Signal0("Jumped", "Jumped fires when a jump starts.")
	sig.wall = b.Signal1[mathx.Vec2]("HitWall", "HitWall fires when the character starts touching a wall.", "normal")
	sig.ceiling = b.Signal0("HitCeiling", "HitCeiling fires when the character bumps into a ceiling.")
	sig.climbed = b.Signal0("Climbed", "Climbed fires when the character starts climbing onto a ledge.")
	return b.Err()
}
