package physics

import (
	"errors"

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
	body := module.Component[components.Body2D](r, schema.Name(components.NameBody), space,
		schema.Doc("Simulates the node as a rigid body. Colliders on the node and its children form its shape."),
		schema.FieldDoc("type", "static never moves, kinematic moves by velocity, dynamic reacts to forces and contacts."),
		schema.FieldDoc("massMode", "auto takes the mass from collider density, override uses Mass."),
		schema.FieldDoc("gravityScale", "Multiplier of the project gravity for this body."),
		schema.FieldDoc("interpolate", "Draws the body between physics steps for smooth motion; scripts still see the exact pose."),
		schema.FieldDoc("layer", "Collision layer, see Project Settings → Physics."),
		schema.FieldDoc("mask", "Layers this body collides with."),
		schema.ChoicesFrom("layer", settings.ID, settings.LayersList),
		schema.Bitset("mask", settings.ID, settings.LayersList),
		schema.Runtime("velocity", "angularVelocity", "awake"),
	).Category(Menu).Display("Rigid Body 2D").Icon("cat.physics").NodeIcon(80).Default(components.DefaultBody2D)
	stdlib.BodyMethods(body)
	return errors.Join(
		body.Err(),
		module.Component[components.BoxCollider2D](r, schema.Name(components.NameBox), space,
			schema.Doc("Box shape in node space.")).
			Category(Menu).Display("Box Collider 2D").Icon("cat.physics").NodeIcon(70).Default(components.DefaultBox).OnAdd(fitBox).
			Validate(validator[components.BoxCollider2D](components.KindBox)).Err(),
		module.Component[components.CircleCollider2D](r, schema.Name(components.NameCircle), space,
			schema.Doc("Circle shape. Needs a uniform node scale.")).
			Category(Menu).Display("Circle Collider 2D").Icon("cat.physics").NodeIcon(70).Default(components.DefaultCircle).OnAdd(fitCircle).
			Validate(validator[components.CircleCollider2D](components.KindCircle)).Err(),
		module.Component[components.CapsuleCollider2D](r, schema.Name(components.NameCapsule), space,
			schema.Doc("Capsule along the local X axis; Length is the full extent including the caps.")).
			Category(Menu).Display("Capsule Collider 2D").Icon("cat.physics").NodeIcon(70).Default(components.DefaultCapsule).OnAdd(fitCapsule).
			Validate(validator[components.CapsuleCollider2D](components.KindCapsule)).Err(),
		module.Component[components.PolygonCollider2D](r, schema.Name(components.NamePolygon), space,
			schema.Doc("Convex polygon of 3 to 8 points.")).
			Category(Menu).Display("Polygon Collider 2D").Icon("cat.physics").NodeIcon(70).Default(components.DefaultPolygon).
			Validate(validator[components.PolygonCollider2D](components.KindPolygon)).Err(),
		module.Component[components.EdgeCollider2D](r, schema.Name(components.NameEdge), space,
			schema.Doc("Polyline for level geometry. OneSided lets bodies pass from below.")).
			Category(Menu).Display("Edge Collider 2D").Icon("cat.physics").NodeIcon(70).Default(components.DefaultEdge).
			Validate(validator[components.EdgeCollider2D](components.KindEdge)).Err(),
		module.Component[components.DistanceJoint2D](r, schema.Name(components.NameDistance), space,
			schema.Doc("Keeps a distance between this body and the Target body. Length 0 uses the distance at start.")).
			Category(Menu).Display("Distance Joint 2D").Default(components.DefaultDistance).Err(),
		module.Component[components.RevoluteJoint2D](r, schema.Name(components.NameRevolute), space,
			schema.Doc("Pins this body to the Target body at Anchor, with an optional limit and motor.")).
			Category(Menu).Display("Revolute Joint 2D").Default(components.DefaultRevolute).Err(),
	)
}
