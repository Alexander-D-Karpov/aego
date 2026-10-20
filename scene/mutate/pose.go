package mutate

import (
	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

func Decompose(m mathx.Mat3) scene.Transform2D {
	sx := mathx.Sqrt(m[0]*m[0] + m[1]*m[1])
	sy := mathx.Sqrt(m[3]*m[3] + m[4]*m[4])
	if m[0]*m[4]-m[1]*m[3] < 0 {
		sy = -sy
	}
	return scene.Transform2D{
		Position: mathx.Vec2{X: m[6], Y: m[7]},
		Rotation: mathx.Atan2(m[1], m[0]),
		Scale:    mathx.Vec2{X: sx, Y: sy},
	}
}

func Unlift(m mathx.Mat4) mathx.Mat3 {
	return mathx.Mat3{m[0], -m[1], 0, -m[4], m[5], 0, m[12], -m[13], 1}
}

func PreserveWorld(s *scene.Scene, h, parent scene.NodeHandle) (scene.Pose, error) {
	s.UpdateTransforms()
	pose := s.Pose(h)
	pw := s.World4(parent)
	if mathx.Abs(pw.Col(0).Cross(pw.Col(1)).Dot(pw.Col(2))) < 1e-8 {
		return pose, errs.New(errs.SceneBadReparent,
			"new parent has a degenerate transform, the child world transform cannot be preserved",
			log.F("parent", s.Name(parent)))
	}
	inv, ok := pw.InverseAffine()
	if !ok {
		return pose, errs.New(errs.SceneBadReparent, "new parent transform cannot be inverted", log.F("parent", s.Name(parent)))
	}
	rel := inv.Mul(s.World4(h))
	if pose.Space == scene.Space3D {
		pos, rot, sc, ok := rel.Decompose()
		if !ok {
			return pose, errs.New(errs.SceneBadReparent, "node has a degenerate transform", log.F("node", s.Name(h)))
		}
		pose.T3 = scene.Transform3D{Position: pos, Rotation: rot, Scale: sc}
		return pose, nil
	}
	pose.T2 = Decompose(Unlift(rel))
	return pose, nil
}

func spaceName(sp scene.Space) string {
	if sp == scene.Space3D {
		return "3d"
	}
	return "2d"
}

func poseBlock(reg *schema.Registry, p scene.Pose) *aetext.Node {
	n := &aetext.Node{Head: "values"}
	put := func(name string, k schema.Kind, v schema.Value) {
		n.Fields = append(n.Fields, aetext.Field{Name: name, Value: sceneio.WriteValue(reg, plain(k), v)})
	}
	if p.Space == scene.Space3D {
		put("position", schema.KindVec3, schema.Vec3(p.T3.Position))
		put("rotation", schema.KindQuat, schema.Quat(p.T3.Rotation))
		put("scale", schema.KindVec3, schema.Vec3(p.T3.Scale))
		return n
	}
	put("position", schema.KindVec2, schema.Vec2(p.T2.Position))
	put("rotation", schema.KindFloat32, schema.Float32(p.T2.Rotation))
	put("scale", schema.KindVec2, schema.Vec2(p.T2.Scale))
	return n
}

func (d *Document) readPose(base scene.Pose, n *aetext.Node) (scene.Pose, error) {
	p := base
	get := func(name string, k schema.Kind, apply func(schema.Value)) error {
		raw, ok := n.Field(name)
		if !ok {
			return nil
		}
		v, err := d.read(plain(k), raw)
		if err != nil {
			return err
		}
		apply(v)
		return nil
	}
	var err error
	if p.Space == scene.Space3D {
		err = firstErr(
			get("position", schema.KindVec3, func(v schema.Value) { p.T3.Position = v.V3 }),
			get("rotation", schema.KindQuat, func(v schema.Value) { p.T3.Rotation = v.Q }),
			get("scale", schema.KindVec3, func(v schema.Value) { p.T3.Scale = v.V3 }),
		)
	} else {
		err = firstErr(
			get("position", schema.KindVec2, func(v schema.Value) { p.T2.Position = v.V }),
			get("rotation", schema.KindFloat32, func(v schema.Value) { p.T2.Rotation = float32(v.F) }),
			get("scale", schema.KindVec2, func(v schema.Value) { p.T2.Scale = v.V }),
		)
	}
	return p, err
}

func firstErr(list ...error) error {
	for _, e := range list {
		if e != nil {
			return e
		}
	}
	return nil
}
