package animedit

import (
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/history"
	"aego/editor/core/history/commands"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/schema"
)

func (s *Service) record(c history.Command) (history.Command, bool) {
	var ops []mutate.Op
	var err error
	switch c := c.(type) {
	case *commands.Mutation:
		ops, err = s.fieldKeys(c)
	case *commands.SetTransform:
		ops, err = s.poseKeys(c)
	default:
		return nil, false
	}
	switch {
	case err != nil:
		s.say(err.Error())
		return nil, true
	case ops == nil:
		return nil, false
	case !s.AutoKey:
		s.say(i18n.Tr("editor.animedit.auto_key_off", "Auto Key is off: press K or the Key button next to the field"))
		return nil, true
	}
	return s.edit(i18n.Tr("editor.history.record_key", "Record Key"), ops), true
}

func (s *Service) edit(label string, ops []mutate.Op) *KeyEdit {
	return &KeyEdit{Name: label, Rel: s.rel, Tx: mutate.Tx{Label: label, Ops: ops}, Files: s.files, Loaded: s.loaded}
}

func (s *Service) Key(node ids.NodeID, path string, kind schema.Kind, v schema.Value) error {
	if !s.applied {
		return errNoClip
	}
	h, rel, ok := s.underID(node)
	if !ok {
		return errs.New(errs.EditorCommand, "the node is not the animated node or below it")
	}
	ops, err := s.keys(h, rel, path, kind, v)
	if err != nil {
		return err
	}
	return s.doc.History.Exec(s.edit(i18n.Tr("editor.history.key", "Key"), ops))
}

func (s *Service) keyTime() string {
	t := s.time
	if fps := s.clip.FPS; fps > 0 {
		t = float32(math.Round(float64(t*fps))) / fps
	}
	return strconv.FormatFloat(float64(t), 'g', -1, 32)
}

func (s *Service) underID(id ids.NodeID) (scene.NodeHandle, string, bool) {
	sc := s.scene
	if sc == nil {
		return scene.NodeHandle{}, "", false
	}
	h, root := sc.Handle(id), sc.Handle(s.target)
	if h.IsZero() || root.IsZero() {
		return h, "", false
	}
	var parts []string
	for n := h; n != root; n = sc.Parent(n) {
		if n.IsZero() || n == sc.Root() {
			return h, "", false
		}
		parts = append(parts, sc.Name(n))
	}
	slices.Reverse(parts)
	return h, strings.Join(parts, "/"), true
}

func (s *Service) fieldKeys(c *commands.Mutation) ([]mutate.Op, error) {
	if len(c.Tx.Ops) != 1 || c.Tx.Ops[0].Kind != mutate.KindSet {
		return nil, nil
	}
	op := c.Tx.Ops[0]
	if !strings.HasPrefix(op.Node, "#") {
		return nil, nil
	}
	id, err := ids.ParseNodeID(op.Node[1:])
	if err != nil {
		return nil, nil
	}
	if !s.applied {
		return nil, errNoClip
	}
	h, rel, ok := s.underID(id)
	if !ok {
		return nil, nil
	}
	i := strings.LastIndexByte(op.Path, '.')
	if i < 0 {
		return nil, nil
	}
	reg := s.scene.Registry()
	td, ok := reg.ByName(op.Path[:i])
	if !ok {
		return nil, nil
	}
	fd, ok := td.Field(op.Path[i+1:])
	if !ok {
		return nil, nil
	}
	v, err := sceneio.ReadValue(reg, fd, op.Value)
	if err != nil {
		return nil, err
	}
	path := schema.ShortName(td.FullName) + "." + fd.Name
	if _, err := s.props.ResolveAny(h, path); err != nil {
		path = td.FullName + "." + fd.Name
	}
	return s.keys(h, rel, path, fd.Kind, v)
}

func (s *Service) poseKeys(c *commands.SetTransform) ([]mutate.Op, error) {
	if !s.applied {
		return nil, errNoClip
	}
	h, rel, ok := s.underID(c.Node)
	if !ok {
		return nil, nil
	}
	var ops []mutate.Op
	add := func(part string, changed bool, kind schema.Kind, v schema.Value) error {
		if !changed {
			return nil
		}
		o, err := s.keys(h, rel, "transform."+part, kind, v)
		ops = append(ops, o...)
		return err
	}
	b, a := c.Before, c.After
	if err := errors.Join(
		add("position", a.Position != b.Position, schema.KindVec2, schema.Vec2(a.Position)),
		add("rotation", a.Rotation != b.Rotation, schema.KindFloat32, schema.Float32(a.Rotation)),
		add("scale", a.Scale != b.Scale, schema.KindVec2, schema.Vec2(a.Scale)),
	); err != nil {
		return nil, err
	}
	return ops, nil
}

func (s *Service) keys(h scene.NodeHandle, rel, path string, kind schema.Kind, v schema.Value) ([]mutate.Op, error) {
	ref, err := s.props.ResolveAny(h, path)
	if err != nil {
		return nil, err
	}
	at := s.keyTime()
	var ops []mutate.Op
	for i := range s.binds {
		b := &s.binds[i]
		if b.node != h || !b.ref.Overlaps(&ref) {
			continue
		}
		prop := s.clip.Tracks[b.track].Property
		if l := b.ref.Lane(); l >= 0 {
			ops = append(ops, mutate.AnimKeySet(rel, prop, at, aetext.Float32(laneValues(v)[l])))
			continue
		}
		kv, ok := keyValue(v)
		if !ok {
			return nil, errs.New(errs.EditorProps, path+" cannot be animated")
		}
		ops = append(ops, mutate.AnimKeySet(rel, prop, at, kv))
	}
	if len(ops) > 0 {
		return ops, nil
	}
	kv, ok := keyValue(v)
	if !ok {
		return nil, errs.New(errs.EditorProps, path+" cannot be animated")
	}
	op := mutate.AnimKeySet(rel, path, at, kv)
	op.Comp = kind.String()
	return []mutate.Op{op}, nil
}

func floats(xs ...float32) aetext.Value {
	out := make([]aetext.Value, len(xs))
	for i, x := range xs {
		out[i] = aetext.Float32(x)
	}
	return aetext.List(out...)
}

func keyValue(v schema.Value) (aetext.Value, bool) {
	switch v.Kind {
	case schema.KindBool:
		return aetext.Bool(v.I != 0), true
	case schema.KindInt32, schema.KindInt64, schema.KindEnum:
		return aetext.Int(v.I), true
	case schema.KindFloat32, schema.KindFloat64:
		return aetext.Float32(float32(v.F)), true
	case schema.KindVec2:
		return floats(v.V.X, v.V.Y), true
	case schema.KindVec3:
		return floats(v.V3.X, v.V3.Y, v.V3.Z), true
	case schema.KindVec4:
		return floats(v.V4.X, v.V4.Y, v.V4.Z, v.V4.W), true
	case schema.KindQuat:
		return floats(v.Q.X, v.Q.Y, v.Q.Z, v.Q.W), true
	case schema.KindColor:
		return floats(v.C.R, v.C.G, v.C.B, v.C.A), true
	case schema.KindRect:
		return floats(v.R.X, v.R.Y, v.R.W, v.R.H), true
	case schema.KindAssetRef:
		return aetext.AssetRef(v.Asset.String()), true
	}
	return aetext.Value{}, false
}

func laneValues(v schema.Value) [4]float32 {
	switch v.Kind {
	case schema.KindFloat32, schema.KindFloat64:
		return [4]float32{float32(v.F)}
	case schema.KindVec2:
		return [4]float32{v.V.X, v.V.Y}
	case schema.KindVec3:
		return [4]float32{v.V3.X, v.V3.Y, v.V3.Z}
	case schema.KindVec4:
		return [4]float32{v.V4.X, v.V4.Y, v.V4.Z, v.V4.W}
	case schema.KindQuat:
		return [4]float32{v.Q.X, v.Q.Y, v.Q.Z, v.Q.W}
	case schema.KindColor:
		return [4]float32{v.C.R, v.C.G, v.C.B, v.C.A}
	case schema.KindRect:
		return [4]float32{v.R.X, v.R.Y, v.R.W, v.R.H}
	}
	return [4]float32{}
}
