package physics

import (
	"aego/core/mathx"
	"aego/engine/module"
	"aego/modules/physics/components"
	"aego/scene"
	"aego/schema"
)

func registerTemplates(r *module.Registry) {
	for _, t := range []struct {
		id    string
		name  string
		icon  string
		setup func(*scene.Scene, scene.NodeHandle)
	}{
		{"rigidbody", "Rigid Body", "comp.rigidbody", rigidBody},
		{"static", "Static Collider", "tpl.static", staticCollider},
		{"trigger", "Trigger", "tpl.trigger", trigger},
		{"platform", "Platform", "tpl.platform", platform},
		{"ball", "Ball", "tpl.ball", ball},
	} {
		r.Template(schema.TemplateDesc{ID: ID + "." + t.id, Name: t.name, Icon: t.icon, Menu: Menu, Space: schema.Space2D, Setup: t.setup})
	}
}

func rigidBody(s *scene.Scene, h scene.NodeHandle) {
	*scene.Add[components.Body2D](s, h) = components.DefaultBody2D()
	*scene.Add[components.BoxCollider2D](s, h) = components.DefaultBox()
}

func staticCollider(s *scene.Scene, h scene.NodeHandle) {
	*scene.Add[components.BoxCollider2D](s, h) = components.DefaultBox()
}

func trigger(s *scene.Scene, h scene.NodeHandle) {
	c := components.DefaultBox()
	c.IsTrigger, c.Density = true, 0
	*scene.Add[components.BoxCollider2D](s, h) = c
}

func platform(s *scene.Scene, h scene.NodeHandle) {
	b := components.DefaultBody2D()
	b.Type = components.Static
	*scene.Add[components.Body2D](s, h) = b
	e := components.DefaultEdge()
	e.OneSided = true
	*scene.Add[components.EdgeCollider2D](s, h) = e
}

func ball(s *scene.Scene, h scene.NodeHandle) {
	*scene.Add[components.Body2D](s, h) = components.DefaultBody2D()
	c := components.DefaultCircle()
	c.Restitution = 0.6
	c.Offset = mathx.Vec2{}
	*scene.Add[components.CircleCollider2D](s, h) = c
}
