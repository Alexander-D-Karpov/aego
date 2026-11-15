package physics

import (
	"errors"
	"slices"

	"aego/engine/module"
	"aego/modules/physics/components"
	"aego/modules/physics/settings"
	"aego/modules/physics/stdlib"
	"aego/schema"
)

func registerTypes(r *module.Registry) error {
	if err := module.Enum[components.BodyType](r, "physics.BodyType",
		schema.EnumValue{Name: "static", Value: int64(components.Static)},
		schema.EnumValue{Name: "kinematic", Value: int64(components.Kinematic)},
		schema.EnumValue{Name: "dynamic", Value: int64(components.Dynamic)},
	); err != nil {
		return err
	}
	if err := module.Enum[components.MassMode](r, "physics.MassMode",
		schema.EnumValue{Name: "auto", Value: int64(components.MassAuto)},
		schema.EnumValue{Name: "override", Value: int64(components.MassOverride)},
	); err != nil {
		return err
	}
	space := schema.Space(schema.Space2D)
	shape := func(opts ...schema.Option) []schema.Option {
		return append([]schema.Option{space,
			schema.FieldDoc("offset", "Position of the shape in node space."),
			schema.FieldDoc("isTrigger", "Detects overlaps and sends trigger events without pushing bodies apart."),
			schema.FieldDoc("friction", "Resistance to sliding along other shapes; 0 is ice. Two shapes use the square root of the product."),
			schema.FieldDoc("restitution", "Bounciness: 0 stops on impact, 1 bounces back at the same speed. Two shapes use the larger value."),
			schema.FieldDoc("density", "Mass per square unit; gives the body its mass when Mass Mode is auto."),
		}, opts...)
	}
	body := module.Component[components.Body2D](r, schema.Name(components.NameBody), space,
		schema.Doc("Simulates the node as a rigid body. Colliders on the node and its children form its shape."),
		schema.FieldDoc("type", "static never moves, kinematic moves by velocity, dynamic reacts to forces and contacts."),
		schema.FieldDoc("massMode", "auto takes the mass from collider density, override uses Mass."),
		schema.FieldDoc("gravityScale", "Multiplier of the project gravity for this body; 0 makes it float."),
		schema.FieldDoc("interpolate", "Draws the body between physics steps for smooth motion; scripts still see the exact pose."),
		schema.FieldDoc("layer", "Collision layer, see Project Settings → Physics."),
		schema.FieldDoc("mask", "Layers this body collides with."),
		schema.FieldDoc("mass", "Mass in kilograms when Mass Mode is override."),
		schema.FieldDoc("linearDamping", "Slows the movement every step, like air resistance; 0 keeps the speed."),
		schema.FieldDoc("angularDamping", "Slows the rotation every step; 0 keeps spinning."),
		schema.FieldDoc("fixedRotation", "Keeps the body upright: contacts and torque never rotate it."),
		schema.FieldDoc("bullet", "Continuous collision against other dynamic bodies, so a fast body cannot pass through them. Costs more."),
		schema.FieldDoc("allowSleep", "Lets the body sleep after it comes to rest, which saves time; contacts and scripts wake it."),
		schema.FieldDoc("velocity", "Velocity in units per second."),
		schema.FieldDoc("angularVelocity", "Rotation speed in radians per second."),
		schema.FieldDoc("awake", "Whether the body is simulated this step or sleeping."),
		schema.ChoicesFrom("layer", settings.ID, settings.LayersList),
		schema.Bitset("mask", settings.ID, settings.LayersList),
		schema.Runtime("velocity", "angularVelocity", "awake"),
	).Category(Menu).Display("Rigid Body 2D").Icon("comp.rigidbody").NodeIcon(80).Default(components.DefaultBody2D)
	stdlib.BodyMethods(body)
	return errors.Join(
		body.Err(),
		module.Component[components.BoxCollider2D](r, shape(schema.Name(components.NameBox),
			schema.Doc("Box shape in node space."),
			schema.FieldDoc("size", "Width and height in units, before the node scale."))...).
			Category(Menu).Display("Box Collider 2D").Icon("comp.box").NodeIcon(70).Default(components.DefaultBox).OnAdd(fitBox).
			Validate(validator[components.BoxCollider2D](components.KindBox)).Err(),
		module.Component[components.CircleCollider2D](r, shape(schema.Name(components.NameCircle),
			schema.Doc("Circle shape. Needs a uniform node scale."),
			schema.FieldDoc("radius", "Radius in units, before the node scale."))...).
			Category(Menu).Display("Circle Collider 2D").Icon("comp.circle").NodeIcon(70).Default(components.DefaultCircle).OnAdd(fitCircle).
			Validate(validator[components.CircleCollider2D](components.KindCircle)).Err(),
		module.Component[components.CapsuleCollider2D](r, shape(schema.Name(components.NameCapsule),
			schema.Doc("Capsule along the local X axis; Length is the full extent including the caps."),
			schema.FieldDoc("length", "Full length along the local X axis, caps included."),
			schema.FieldDoc("radius", "Radius of the caps and half the thickness."))...).
			Category(Menu).Display("Capsule Collider 2D").Icon("comp.capsule").NodeIcon(70).Default(components.DefaultCapsule).OnAdd(fitCapsule).
			Validate(validator[components.CapsuleCollider2D](components.KindCapsule)).Err(),
		module.Component[components.PolygonCollider2D](r, shape(schema.Name(components.NamePolygon),
			schema.Doc("Convex polygon of 3 to 8 points."),
			schema.FieldDoc("points", "Corners in node space, 3 to 8, forming a convex shape; edit them in the Scene View."))...).
			Category(Menu).Display("Polygon Collider 2D").Icon("comp.polygon").NodeIcon(70).Default(components.DefaultPolygon).
			Validate(validator[components.PolygonCollider2D](components.KindPolygon)).Err(),
		module.Component[components.EdgeCollider2D](r, shape(schema.Name(components.NameEdge),
			schema.FullClone(cloneEdgeCollider),
			schema.Doc("Polyline for level geometry. OneSided lets bodies pass from below."),
			schema.FieldDoc("points", "Points of the line in node space, at least 2; edit them in the Scene View."),
			schema.FieldDoc("loop", "Joins the last point to the first, for closed outlines; needs at least 3 points."),
			schema.FieldDoc("oneSided", "Collides only from above, so bodies can jump through from below, as on a platform."))...).
			Category(Menu).Display("Edge Collider 2D").Icon("comp.edge").NodeIcon(70).Default(components.DefaultEdge).
			Validate(validator[components.EdgeCollider2D](components.KindEdge)).Err(),
		module.Component[components.DistanceJoint2D](r, schema.Name(components.NameDistance), space,
			schema.Doc("Keeps a distance between this body and the Target body. Length 0 uses the distance at start."),
			schema.FieldDoc("target", "The other body; both nodes need a Rigid Body 2D."),
			schema.FieldDoc("anchorA", "Attachment point on this node, in node space."),
			schema.FieldDoc("anchorB", "Attachment point on the Target node, in its node space."),
			schema.FieldDoc("length", "Rest length in units; 0 takes the distance between the anchors at start."),
			schema.FieldDoc("minLength", "Shortest allowed length. With Min and Max both 0 the joint is a rigid rod, or a spring without a lower limit when Stiffness is set."),
			schema.FieldDoc("maxLength", "Longest allowed length; a rope has Min 0 and Max at the rope length."),
			schema.FieldDoc("stiffness", "Spring stiffness in newtons per unit; 0 makes the length rigid within Min and Max."),
			schema.FieldDoc("damping", "Spring damping; 0 never damps, higher values stop the oscillation sooner."),
			schema.FieldDoc("collideConnected", "Lets the two connected bodies collide with each other.")).
			Category(Menu).Display("Distance Joint 2D").Icon("comp.joint_distance").Default(components.DefaultDistance).Err(),
		module.Component[components.RevoluteJoint2D](r, schema.Name(components.NameRevolute), space,
			schema.Doc("Pins this body to the Target body at Anchor, with an optional limit and motor."),
			schema.FieldDoc("target", "The body this one turns around; both nodes need a Rigid Body 2D."),
			schema.FieldDoc("anchor", "Pivot point in this node's space; both bodies turn around it."),
			schema.FieldDoc("enableLimit", "Keeps the angle between Lower and Upper."),
			schema.FieldDoc("lower", "Smallest angle in radians, relative to the angle at start."),
			schema.FieldDoc("upper", "Largest angle in radians, relative to the angle at start."),
			schema.FieldDoc("enableMotor", "Drives the joint at Speed, up to Max Torque."),
			schema.FieldDoc("speed", "Motor speed in radians per second; negative turns the other way."),
			schema.FieldDoc("maxTorque", "Strongest torque the motor may apply; with 0 the motor does nothing."),
			schema.FieldDoc("collideConnected", "Lets the two connected bodies collide with each other.")).
			Category(Menu).Display("Revolute Joint 2D").Icon("comp.joint_revolute").Default(components.DefaultRevolute).Err(),
	)
}

func cloneEdgeCollider(dst, src *components.EdgeCollider2D) {
	*dst = *src
	dst.Points = slices.Clone(src.Points)
}
