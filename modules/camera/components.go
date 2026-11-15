package camera

import (
	"errors"

	"aego/core/curve"
	"aego/core/mathx"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
)

const (
	NameVirtual = "camera.VirtualCamera2D"
	NameFollow  = "camera.CameraFollow2D"
	NameShake   = "camera.CameraShake2D"
	NameZone    = "camera.CameraZone2D"
	NameBrain   = "camera.CameraBrain2D"
	Category    = "Camera"

	cameraComponent = "engine.Camera2D"

	CodeNoVirtual = "CAM001"
	CodeNoCamera  = "CAM002"
	CodeEmptyRect = "CAM003"
	CodeNoTarget  = "CAM004"
)

type TimeDomain uint8

const (
	Unscaled TimeDomain = iota
	Game
)

type VirtualCamera2D struct {
	Priority     int32       `aego:"priority"`
	Output       string      `aego:"output"`
	Zoom         float32     `aego:"zoom,min=0"`
	BlendTime    float32     `aego:"blendTime,min=0"`
	BlendCurve   curve.Curve `aego:"blendCurve"`
	TimeDomain   TimeDomain  `aego:"timeDomain"`
	UseBounds    bool        `aego:"useBounds"`
	Bounds       mathx.Rect  `aego:"bounds"`
	SnapToPixels bool        `aego:"snapToPixels"`
}

type CameraFollow2D struct {
	Target             scene.NodeRef `aego:"target"`
	Deadzone           mathx.Rect    `aego:"deadzone"`
	Damping            mathx.Vec2    `aego:"damping"`
	Lookahead          mathx.Vec2    `aego:"lookahead"`
	MaxLookahead       mathx.Vec2    `aego:"maxLookahead"`
	LookaheadSmoothing float32       `aego:"lookaheadSmoothing,min=0"`
	SnapDistance       float32       `aego:"snapDistance,min=0"`
}

type CameraShake2D struct {
	Decay     float32    `aego:"decay,min=0"`
	MaxOffset mathx.Vec2 `aego:"maxOffset"`
	MaxAngle  float32    `aego:"maxAngle,min=0"`
	Frequency float32    `aego:"frequency,min=0"`
	Exponent  float32    `aego:"exponent,min=1"`
}

type CameraZone2D struct {
	Target    scene.NodeRef `aego:"target"`
	Camera    scene.NodeRef `aego:"camera"`
	Priority  int32         `aego:"priority"`
	Zoom      float32       `aego:"zoom,min=0"`
	UseBounds bool          `aego:"useBounds"`
	Bounds    mathx.Rect    `aego:"bounds"`
	Area      mathx.Rect    `aego:"area"`
}

type CameraBrain2D struct {
	Output string `aego:"output"`
}

func DefaultVirtual() VirtualCamera2D {
	return VirtualCamera2D{BlendTime: 0.5, BlendCurve: curve.FromEasing(curve.InOutQuad)}
}

func DefaultFollow() CameraFollow2D {
	return CameraFollow2D{
		Deadzone:           mathx.Rect{X: 0.4, Y: 0.4, W: 0.2, H: 0.2},
		Damping:            mathx.Vec2{X: 6, Y: 6},
		Lookahead:          mathx.Vec2{X: 0.25},
		MaxLookahead:       mathx.Vec2{X: 3, Y: 1},
		LookaheadSmoothing: 3,
		SnapDistance:       20,
	}
}

func DefaultShake() CameraShake2D {
	return CameraShake2D{Decay: 1.5, MaxOffset: mathx.Vec2{X: 0.5, Y: 0.5}, MaxAngle: 0.05, Frequency: 25, Exponent: 2}
}

func DefaultZone() CameraZone2D {
	return CameraZone2D{Priority: 10, Area: mathx.Rect{X: -4, Y: -3, W: 8, H: 6}}
}

func diag(code string, sev problems.Severity, msg string) problems.Diagnostic {
	return problems.Diagnostic{Code: code, Severity: sev, Message: msg}
}

func empty(r mathx.Rect) bool {
	return r.W <= 0 || r.H <= 0
}

func validateVirtual(_ module.ValidateContext, vc *VirtualCamera2D) []problems.Diagnostic {
	if vc.UseBounds && empty(vc.Bounds) {
		return []problems.Diagnostic{diag(CodeEmptyRect, problems.Warning, "Virtual Camera 2D: Bounds has no area, the camera is not clamped")}
	}
	return nil
}

func validateFollow(ctx module.ValidateContext, f *CameraFollow2D) []problems.Diagnostic {
	var out []problems.Diagnostic
	if !scene.Has[VirtualCamera2D](ctx.Scene, ctx.Node) {
		out = append(out, diag(CodeNoVirtual, problems.Warning, "Camera Follow 2D needs a Virtual Camera 2D on the same node"))
	}
	if f.Target.IsZero() {
		out = append(out, diag(CodeNoTarget, problems.Warning, "Camera Follow 2D has no Target"))
	}
	return out
}

func validateShake(ctx module.ValidateContext, _ *CameraShake2D) []problems.Diagnostic {
	if !scene.Has[VirtualCamera2D](ctx.Scene, ctx.Node) {
		return []problems.Diagnostic{diag(CodeNoVirtual, problems.Warning, "Camera Shake 2D needs a Virtual Camera 2D on the same node")}
	}
	return nil
}

func validateZone(ctx module.ValidateContext, z *CameraZone2D) []problems.Diagnostic {
	var out []problems.Diagnostic
	if z.Camera.IsZero() && !scene.Has[VirtualCamera2D](ctx.Scene, ctx.Node) {
		out = append(out, diag(CodeNoCamera, problems.Warning, "Camera Zone 2D needs a Camera or a Virtual Camera 2D on the same node"))
	}
	if empty(z.Area) {
		out = append(out, diag(CodeEmptyRect, problems.Warning, "Camera Zone 2D: Area has no size, the zone never triggers"))
	}
	if z.UseBounds && empty(z.Bounds) {
		out = append(out, diag(CodeEmptyRect, problems.Warning, "Camera Zone 2D: Bounds has no area"))
	}
	return out
}

func followCamera(s *scene.Scene, h scene.NodeHandle) {
	reg := s.Registry()
	for _, name := range [...]string{cameraComponent, NameVirtual, NameFollow} {
		if td, ok := reg.ByName(name); ok {
			module.Ensure(s, h, td)
		}
	}
}

func registerTypes(r *module.Registry) error {
	if err := module.Enum[TimeDomain](r, "camera.TimeDomain",
		schema.EnumValue{Name: "unscaled", Value: int64(Unscaled)},
		schema.EnumValue{Name: "game", Value: int64(Game)},
	); err != nil {
		return err
	}
	space := schema.Space(schema.Space2D)
	vc := module.Component[VirtualCamera2D](r, schema.Name(NameVirtual), space,
		schema.Doc("A camera pose the brain can pick. The highest priority wins; switching blends from the current view."),
		schema.FieldDoc("priority", "The enabled virtual camera with the highest priority drives its output."),
		schema.FieldDoc("output", "Output this camera drives. Empty is the main output; a Camera 2D picks another one with Camera Output Selector 2D."),
		schema.FieldDoc("zoom", "Pixels per world unit while this camera is active. 0 keeps the zoom of the Camera 2D."),
		schema.FieldDoc("blendTime", "Seconds to blend into this camera when it becomes active. 0 cuts."),
		schema.FieldDoc("blendCurve", "Shape of the blend; its time range is stretched over Blend Time."),
		schema.FieldDoc("timeDomain", "unscaled blends and shakes in real time, also during hit-stop and pause; game follows the time scale."),
		schema.FieldDoc("useBounds", "Keeps the visible area inside Bounds."),
		schema.FieldDoc("bounds", "World rectangle the view stays inside. A smaller area centres the view on it."),
		schema.FieldDoc("snapToPixels", "Rounds the final position to whole screen pixels. pixelPerfect cameras already do this."),
	).Suggests(NameFollow).Category(Category).Display("Virtual Camera 2D").Icon("comp.vcam").NodeIcon(90).Default(DefaultVirtual).Validate(validateVirtual)
	vc.Method0("IsActive", func(c *bind.Ctx, _ *VirtualCamera2D) bool {
		s := live(c)
		return s != nil && s.IsActive(c.Recv)
	}, bind.Doc("IsActive reports whether this virtual camera drives its output now."))

	follow := module.Component[CameraFollow2D](r, schema.Name(NameFollow), space,
		schema.Doc("Moves its virtual camera after Target, with a deadzone, damping and lookahead."),
		schema.FieldDoc("target", "Node to follow; its interpolated position is used."),
		schema.FieldDoc("deadzone", "Area of the view, in 0..1 of its width and height, where the target moves without moving the camera."),
		schema.FieldDoc("damping", "How fast the camera catches up, per second on each axis. 0 follows rigidly."),
		schema.FieldDoc("lookahead", "Seconds of target velocity the camera looks ahead on each axis."),
		schema.FieldDoc("maxLookahead", "Largest lookahead in world units on each axis. 0 is unlimited."),
		schema.FieldDoc("lookaheadSmoothing", "How fast the lookahead changes, per second. 0 changes at once."),
		schema.FieldDoc("snapDistance", "A target jump longer than this, in world units, moves the camera at once. 0 never snaps."),
	).Requires(NameVirtual).Category(Category).Display("Camera Follow 2D").Icon("comp.follow").Default(DefaultFollow).Validate(validateFollow)
	follow.Proc0("Snap", func(c *bind.Ctx, _ *CameraFollow2D) {
		if s := live(c); s != nil {
			s.Snap(c.Recv)
		}
	}, bind.Doc("Snap moves the camera onto the target at once, for example after a teleport."))

	shake := module.Component[CameraShake2D](r, schema.Name(NameShake), space,
		schema.Doc("Tunes the shake of its virtual camera. Shake grows with trauma and fades over time."),
		schema.FieldDoc("decay", "Trauma lost per second."),
		schema.FieldDoc("maxOffset", "Largest offset in world units at full trauma."),
		schema.FieldDoc("maxAngle", "Largest rotation in radians at full trauma."),
		schema.FieldDoc("frequency", "How fast the shake changes, in noise steps per second."),
		schema.FieldDoc("exponent", "Shake is trauma to this power: 2 keeps small hits subtle."),
	).Requires(NameVirtual).Category(Category).Display("Camera Shake 2D").Icon("comp.shake").Default(DefaultShake).Validate(validateShake)
	shake.Proc1("Add", func(c *bind.Ctx, _ *CameraShake2D, t float32) {
		if s := live(c); s != nil {
			s.ShakeNode(c.Recv, t)
		}
	}, bind.Doc("Add adds trauma to the output of this camera; trauma is capped at 1."), bind.Params("trauma"))

	zone := module.Component[CameraZone2D](r, schema.Name(NameZone), space,
		schema.Doc("While Target is inside Area, Camera gets Priority and the zone's zoom and bounds."),
		schema.FieldDoc("target", "Node that triggers the zone. Empty uses the follow target of the zone's camera."),
		schema.FieldDoc("camera", "Virtual camera the zone activates. Empty uses the one on this node."),
		schema.FieldDoc("priority", "Priority of the camera while the target is inside."),
		schema.FieldDoc("zoom", "Zoom while the zone is in effect. 0 keeps the camera's zoom."),
		schema.FieldDoc("useBounds", "Replaces the camera's bounds while the zone is in effect."),
		schema.FieldDoc("bounds", "World rectangle used instead of the camera's bounds."),
		schema.FieldDoc("area", "Trigger rectangle in this node's space."),
	).Category(Category).Display("Camera Zone 2D").Icon("comp.zone").Default(DefaultZone).Validate(validateZone)

	brain := module.Component[CameraBrain2D](r, schema.Name(NameBrain), space,
		schema.Doc("Picks which output of virtual cameras drives this Camera 2D. Without it the camera shows the main output."),
		schema.FieldDoc("output", "Output name; virtual cameras with the same Output drive this camera."),
	).Category(Category).Display("Camera Output Selector 2D").SearchAliases("Camera Brain 2D", "Мозг камеры 2D").Icon("comp.camera2d")

	r.Template(schema.TemplateDesc{ID: ID + ".follow", Name: "Follow Camera", Icon: "comp.follow", Category: Category, Space: schema.Space2D, Setup: followCamera})
	return errors.Join(vc.Err(), follow.Err(), shake.Err(), zone.Err(), brain.Err())
}
