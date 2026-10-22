package mutate

import (
	"errors"
	"slices"
	"strconv"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/scene/scriptcomp"
	"aego/schema"
)

var ErrNothing = errors.New("nothing matches")

type Change struct {
	Op     string         `json:"op"`
	Node   string         `json:"node"`
	Path   string         `json:"path,omitempty"`
	Before string         `json:"before,omitempty"`
	After  string         `json:"after,omitempty"`
	NodeID ids.NodeID     `json:"-"`
	Type   schema.TypeID  `json:"-"`
	Field  schema.FieldID `json:"-"`
}

type Result struct {
	Inverse  Tx        `json:"-"`
	Redo     Tx        `json:"-"`
	Changes  []Change  `json:"changes"`
	Problems []Problem `json:"problems"`
	Created  []string  `json:"created,omitempty"`
	DryRun   bool      `json:"dryRun,omitempty"`
}

type OpError struct {
	Index int
	Kind  string
	Err   error
}

func (e *OpError) Error() string {
	return "operation " + strconv.Itoa(e.Index+1) + " (" + e.Kind + "): " + e.Err.Error()
}

func (e *OpError) Unwrap() error {
	return e.Err
}

type step struct {
	inv    Op
	redo   Op
	change Change
}

type doer interface {
	do(op Op, res *Result) (step, error)
}

func run(d doer, tx Tx) (*Result, error) {
	res := &Result{Inverse: Tx{Label: tx.Label}, Redo: Tx{Label: tx.Label}}
	var inv []Op
	for i, op := range tx.Ops {
		st, err := d.do(op, res)
		if err != nil {
			slices.Reverse(inv)
			replay(d, inv)
			return nil, &OpError{Index: i, Kind: op.Kind, Err: err}
		}
		inv = append(inv, st.inv)
		res.Redo.Ops = append(res.Redo.Ops, st.redo)
		res.Changes = append(res.Changes, st.change)
	}
	slices.Reverse(inv)
	res.Inverse.Ops = inv
	return res, nil
}

func replay(d doer, ops []Op) {
	var scratch Result
	for _, op := range ops {
		d.do(op, &scratch)
	}
}

func dryRun(d doer, tx Tx) (*Result, error) {
	res, err := run(d, tx)
	if err != nil {
		return nil, err
	}
	replay(d, res.Inverse.Ops)
	res.Inverse, res.Redo, res.DryRun = Tx{}, Tx{}, true
	return res, nil
}

func (d *Document) Apply(tx Tx) (*Result, error) {
	return run(d, tx)
}

func (d *Document) DryRun(tx Tx) (*Result, error) {
	return dryRun(d, tx)
}

func (d *Document) do(op Op, res *Result) (step, error) {
	switch op.Kind {
	case KindAddNode:
		return d.addNode(op, res)
	case KindRemoveNode:
		return d.removeNode(op)
	case KindRestoreNode, KindInsert:
		return d.insert(op, res)
	case KindReplace:
		h, err := d.node(op.Node)
		if err != nil {
			return step{}, err
		}
		st, _, err := d.replaceWith(h, []byte(op.Fragment), op.Kind)
		return st, err
	case KindMove:
		return d.move(op)
	case KindRename:
		return d.rename(op)
	case KindAddComp:
		return d.addComp(op, res)
	case KindRemoveComp:
		return d.removeComp(op, res)
	case KindSet:
		v, err := op.value()
		if err != nil {
			return step{}, err
		}
		return d.set(op, v, false)
	case KindReset:
		return d.set(op, aetext.Value{}, true)
	case KindPrefabInstantiate:
		return d.instantiate(op, res)
	case KindPrefabRevert:
		return d.revertPrefab(op)
	case KindPrefabUnpack:
		return d.unpack(op)
	}
	if dataKinds[op.Kind] {
		return step{}, errs.New(errs.EditorCommand, op.Kind+" applies to data assets, not scenes")
	}
	return step{}, errs.New(errs.EditorCommand, "unknown operation "+strconv.Quote(op.Kind))
}

func childIndex(s *scene.Scene, h scene.NodeHandle) int {
	i, idx := 0, -1
	s.Children(s.Parent(h), func(c scene.NodeHandle) bool {
		if c == h {
			idx = i
			return false
		}
		i++
		return true
	})
	return idx
}

func (d *Document) place(h, parent scene.NodeHandle, op Op) error {
	if !op.HasIndex {
		return nil
	}
	if err := d.Scene.Reparent(h, parent, op.Index); err != nil {
		return err
	}
	d.Scene.Flush()
	return nil
}

func (d *Document) change(kind string, h scene.NodeHandle) Change {
	return Change{Op: kind, Node: d.nodeName(h), NodeID: d.Scene.ID(h)}
}

func (d *Document) addNode(op Op, res *Result) (step, error) {
	s := d.Scene
	p, err := d.node(op.Parent)
	if err != nil {
		return step{}, err
	}
	if op.Name == "" {
		return step{}, errs.New(errs.EditorCommand, "add-node needs a name")
	}
	var h scene.NodeHandle
	switch {
	case op.ID != "":
		id, err := ids.ParseNodeID(op.ID)
		if err != nil {
			return step{}, errs.Wrap(err, errs.EditorCommand, "bad node id")
		}
		sp := s.Space(p)
		switch op.Space {
		case "2d":
			sp = scene.Space2D
		case "3d":
			sp = scene.Space3D
		}
		if h, err = s.CreateIn(p, op.Name, id, sp); err != nil {
			return step{}, err
		}
	case op.Space == "2d":
		h = s.Create2D(p, op.Name)
	case op.Space == "3d":
		h = s.Create3D(p, op.Name)
	case op.Space == "":
		h = s.Create(p, op.Name)
	default:
		return step{}, errs.New(errs.EditorCommand, "space is 2d or 3d", log.F("space", op.Space))
	}
	s.Flush()
	if err := d.place(h, p, op); err != nil {
		s.Destroy(h)
		s.Flush()
		return step{}, err
	}
	res.Created = append(res.Created, idRef(s, h))
	redo := Op{Kind: KindAddNode, Parent: idRef(s, p), Name: s.Name(h), ID: s.ID(h).String(), Space: spaceName(s.Space(h)),
		Index: childIndex(s, h), HasIndex: true}
	ch := d.change(op.Kind, h)
	ch.After = idRef(s, h)
	return step{inv: RemoveNode(idRef(s, h)), redo: redo, change: ch}, nil
}

func (d *Document) removeNode(op Op) (step, error) {
	s := d.Scene
	h, err := d.node(op.Node)
	if err != nil {
		return step{}, err
	}
	if h == s.Root() {
		return step{}, errs.New(errs.SceneBadReparent, "the root node cannot be removed")
	}
	ch := d.change(op.Kind, h)
	inv := Op{Kind: KindInsert, Parent: idRef(s, s.Parent(h)), Index: childIndex(s, h), HasIndex: true,
		Fragment: string(sceneio.SaveSubtree(s, h, false))}
	redo := RemoveNode(idRef(s, h))
	s.Destroy(h)
	s.Flush()
	return step{inv: inv, redo: redo, change: ch}, nil
}

func (d *Document) load(p scene.NodeHandle, fragment []byte, name string) (scene.NodeHandle, error) {
	s := d.Scene
	created, _, err := sceneio.LoadInto(s, p, fragment, name, sceneio.LoadOptions{KeepIDs: true, Prefabs: d.env.Library})
	if err != nil {
		return scene.NodeHandle{}, err
	}
	var root scene.NodeHandle
	for _, h := range created {
		if s.Parent(h) != p {
			continue
		}
		if !root.IsZero() {
			return root, errs.New(errs.EditorCommand, "a fragment must have one root node")
		}
		root = h
	}
	if root.IsZero() {
		return root, errs.New(errs.SceneIOParse, "the fragment holds no node")
	}
	return root, nil
}

func (d *Document) insert(op Op, res *Result) (step, error) {
	s := d.Scene
	p, err := d.node(op.Parent)
	if err != nil {
		return step{}, err
	}
	h, err := d.load(p, []byte(op.Fragment), "insert")
	if err != nil {
		return step{}, err
	}
	if err := d.place(h, p, op); err != nil {
		return step{}, err
	}
	res.Created = append(res.Created, idRef(s, h))
	redo := Op{Kind: KindInsert, Parent: idRef(s, p), Fragment: op.Fragment, Index: childIndex(s, h), HasIndex: true}
	return step{inv: RemoveNode(idRef(s, h)), redo: redo, change: d.change(KindInsert, h)}, nil
}

func (d *Document) subtree(h scene.NodeHandle) []byte {
	return sceneio.SaveSubtreeWith(d.Scene, h, d.library())
}

func (d *Document) replaceWith(h scene.NodeHandle, after []byte, kind string) (step, scene.NodeHandle, error) {
	s := d.Scene
	if h == s.Root() {
		return step{}, h, errs.New(errs.SceneBadReparent, "the root node cannot be replaced")
	}
	parent, index, oldID := s.Parent(h), childIndex(s, h), idRef(s, h)
	before := d.subtree(h)
	s.Destroy(h)
	s.Flush()
	nh, err := d.load(parent, after, "replace")
	if err == nil {
		err = s.Reparent(nh, parent, index)
		s.Flush()
	}
	if err != nil {
		if back, berr := d.load(parent, before, "restore"); berr == nil {
			s.Reparent(back, parent, index)
			s.Flush()
		}
		return step{}, h, err
	}
	ch := d.change(kind, nh)
	return step{inv: Replace(idRef(s, nh), string(before)), redo: Replace(oldID, string(after)), change: ch}, nh, nil
}

func (d *Document) move(op Op) (step, error) {
	s := d.Scene
	h, err := d.node(op.Node)
	if err != nil {
		return step{}, err
	}
	np, err := d.node(op.Parent)
	if err != nil {
		return step{}, err
	}
	before := d.nodeName(h)
	old := s.Pose(h)
	inv := Op{Kind: KindMove, Node: idRef(s, h), Parent: idRef(s, s.Parent(h)), Index: childIndex(s, h), HasIndex: true, Values: poseBlock(d.reg, old)}
	pose, keep := old, false
	switch {
	case op.Values != nil:
		if pose, err = d.readPose(old, op.Values); err != nil {
			return step{}, err
		}
		keep = true
	case op.KeepWorld:
		if pose, err = PreserveWorld(s, h, np); err != nil {
			return step{}, err
		}
		keep = true
	}
	index := -1
	if op.HasIndex {
		index = op.Index
	}
	if err := s.Reparent(h, np, index); err != nil {
		return step{}, err
	}
	s.Flush()
	if keep {
		s.SetPose(h, pose)
	}
	redo := Op{Kind: KindMove, Node: idRef(s, h), Parent: idRef(s, np), Index: childIndex(s, h), HasIndex: true, Values: poseBlock(d.reg, s.Pose(h))}
	ch := d.change(op.Kind, h)
	ch.Node, ch.After = before, d.nodeName(h)
	return step{inv: inv, redo: redo, change: ch}, nil
}

func (d *Document) rename(op Op) (step, error) {
	s := d.Scene
	h, err := d.node(op.Node)
	if err != nil {
		return step{}, err
	}
	if h == s.Root() || op.Name == "" {
		return step{}, errs.New(errs.EditorCommand, "rename needs a node below the root and a name")
	}
	before := s.Name(h)
	after := s.SetName(h, op.Name)
	ch := d.change(op.Kind, h)
	ch.Before, ch.After = before, after
	return step{inv: Rename(idRef(s, h), before), redo: Rename(idRef(s, h), after), change: ch}, nil
}

func (d *Document) values(td *schema.TypeDescriptor, p *aetext.Node, dst func(fd *schema.FieldDescriptor, v schema.Value) error) error {
	if p == nil {
		return nil
	}
	for _, f := range p.Fields {
		fd, ok := field(td, f.Name)
		if !ok {
			return errs.New(errs.EditorProps, td.FullName+" has no field "+f.Name)
		}
		v, err := d.read(fd, f.Value)
		if err != nil {
			return err
		}
		if err := dst(fd, v); err != nil {
			return err
		}
	}
	return nil
}

func (d *Document) snapshot(td *schema.TypeDescriptor, h scene.NodeHandle) *aetext.Node {
	p, _ := d.Scene.GetByType(h, td.ID)
	n := &aetext.Node{Head: "values"}
	td.Serializable(func(fd *schema.FieldDescriptor) {
		var v schema.Value
		fd.Get(p, &v)
		n.Fields = append(n.Fields, aetext.Field{Name: fd.Name, Value: sceneio.WriteValue(d.reg, fd, v)})
	})
	return n
}

func (d *Document) addComp(op Op, res *Result) (step, error) {
	s := d.Scene
	h, err := d.node(op.Node)
	if err != nil {
		return step{}, err
	}
	td, err := d.component(op.Comp)
	if err != nil {
		return step{}, err
	}
	if _, ok := s.GetByType(h, td.ID); ok {
		return step{}, errs.New(errs.EditorProps, "node already has "+td.FullName)
	}
	if !td.Space.Allows(s.Space(h).Kind()) {
		return step{}, errs.New(errs.SceneSpaceMismatch, td.FullName+" does not fit the node space", log.F("space", s.Space(h).String()))
	}
	p, err := s.AddByType(h, td.ID)
	if err != nil {
		return step{}, err
	}
	if !op.Raw && td.Default != nil && td.Copy != nil {
		td.Copy(p, td.Default())
	}
	err = d.values(td, op.Values, func(fd *schema.FieldDescriptor, v schema.Value) error {
		if !op.Raw {
			if err := d.validate(fd, v, td.FullName+"."+fd.Name, op.Live); err != nil {
				return err
			}
		}
		return fd.Set(p, v)
	})
	if err != nil {
		s.RemoveByType(h, td.ID)
		s.Flush()
		return step{}, err
	}
	if !op.Raw && d.env.OnAdd != nil {
		d.env.OnAdd(td, s, h)
	}
	s.Flush()
	for _, req := range td.Requires {
		if rt, ok := d.reg.ByName(req); ok {
			if _, has := s.GetByType(h, rt.ID); !has {
				res.Problems = append(res.Problems, Problem{Node: d.nodeName(h), Message: td.FullName + " needs " + req + " on the same node"})
			}
		}
	}
	redo := Op{Kind: KindAddComp, Node: idRef(s, h), Comp: td.FullName, Raw: true, Values: d.snapshot(td, h)}
	ch := d.change(op.Kind, h)
	ch.Path, ch.Type = td.FullName, td.ID
	return step{inv: RemoveComp(idRef(s, h), td.FullName), redo: redo, change: ch}, nil
}

func (d *Document) removeComp(op Op, res *Result) (step, error) {
	s := d.Scene
	h, err := d.node(op.Node)
	if err != nil {
		return step{}, err
	}
	td, err := d.component(op.Comp)
	if err != nil {
		return step{}, err
	}
	if _, ok := s.GetByType(h, td.ID); !ok {
		return step{}, errs.New(errs.SceneNoComponent, "node has no "+td.FullName)
	}
	inv := Op{Kind: KindAddComp, Node: idRef(s, h), Comp: td.FullName, Raw: true, Values: d.snapshot(td, h)}
	if td.ID == scriptcomp.ScriptTypeID {
		inv = Replace(idRef(s, h), string(d.subtree(h)))
	}
	if d.env.OnRemove != nil {
		d.env.OnRemove(td, s, h)
	}
	s.RemoveByType(h, td.ID)
	s.Flush()
	for _, other := range d.reg.Components() {
		if _, has := s.GetByType(h, other.ID); has && slices.Contains(other.Requires, td.FullName) {
			res.Problems = append(res.Problems, Problem{Node: d.nodeName(h), Message: other.FullName + " needs " + td.FullName + " on the same node"})
		}
	}
	ch := d.change(op.Kind, h)
	ch.Path, ch.Type = td.FullName, td.ID
	return step{inv: inv, redo: RemoveComp(idRef(s, h), td.FullName), change: ch}, nil
}

func defaultOf(td *schema.TypeDescriptor, fd *schema.FieldDescriptor) schema.Value {
	var v schema.Value
	switch {
	case td.Default != nil:
		fd.Get(td.Default(), &v)
	case td.New != nil:
		fd.Get(td.New(), &v)
	case fd.HasDefault:
		v = fd.Default
	default:
		v.Kind = fd.Kind
	}
	return v
}

func (d *Document) next(fd *schema.FieldDescriptor, cur schema.Value, raw aetext.Value, lane string, index int, path string) (schema.Value, error) {
	switch {
	case lane != "":
		return setLane(cur, lane, raw)
	case index >= 0:
		if fd.Kind != schema.KindList || index >= len(cur.List) {
			return cur, errs.New(errs.EditorProps, "index out of range", log.F("path", path), log.F("len", len(cur.List)))
		}
		el, err := d.read(elemOf(fd), raw)
		if err != nil {
			return cur, err
		}
		next := cur
		next.List = slices.Clone(cur.List)
		next.List[index] = el
		return next, nil
	}
	return d.read(fd, raw)
}

func (d *Document) set(op Op, raw aetext.Value, reset bool) (step, error) {
	t, err := d.target(op.Node, op.Path)
	if err != nil {
		return step{}, err
	}
	if t.pose != "" {
		return d.setPose(op, t, raw, reset)
	}
	s := d.Scene
	cur, err := s.GetProperty(t.h, t.td.ID, t.fd.ID)
	if err != nil {
		return step{}, errs.New(errs.SceneNoComponent, "node has no "+t.td.FullName, log.F("node", d.nodeName(t.h)))
	}
	var nv schema.Value
	if reset {
		nv = defaultOf(t.td, t.fd)
	} else if nv, err = d.next(t.fd, cur, raw, t.lane, t.index, op.Path); err != nil {
		return step{}, err
	}
	if err := d.validate(t.fd, nv, t.base, op.Live); err != nil {
		return step{}, err
	}
	if err := s.SetProperty(t.h, t.td.ID, t.fd.ID, nv); err != nil {
		return step{}, err
	}
	inv := SetValue(idRef(s, t.h), t.base, sceneio.WriteValue(d.reg, t.fd, cur))
	redo := SetValue(idRef(s, t.h), t.base, sceneio.WriteValue(d.reg, t.fd, nv))
	inv.Live, redo.Live = op.Live, op.Live
	ch := d.change(op.Kind, t.h)
	ch.Path, ch.Before, ch.After = t.base, d.text(t.fd, cur), d.text(t.fd, nv)
	ch.Type, ch.Field = t.td.ID, t.fd.ID
	return step{inv: inv, redo: redo, change: ch}, nil
}

func (d *Document) poseValue(t target) (schema.Value, *schema.FieldDescriptor) {
	s := d.Scene
	if t.pose == pathEnabled {
		return schema.Bool(s.Enabled(t.h)), plain(schema.KindBool)
	}
	pose := s.Pose(t.h)
	if pose.Space == scene.Space3D {
		switch t.pose {
		case "position":
			return schema.Vec3(pose.T3.Position), plain(schema.KindVec3)
		case "rotation":
			return schema.Quat(pose.T3.Rotation), plain(schema.KindQuat)
		}
		return schema.Vec3(pose.T3.Scale), plain(schema.KindVec3)
	}
	switch t.pose {
	case "position":
		return schema.Vec2(pose.T2.Position), plain(schema.KindVec2)
	case "rotation":
		return schema.Float32(pose.T2.Rotation), plain(schema.KindFloat32)
	}
	return schema.Vec2(pose.T2.Scale), plain(schema.KindVec2)
}

func (d *Document) writePose(t target, v schema.Value) {
	s := d.Scene
	if t.pose == pathEnabled {
		s.SetEnabled(t.h, v.I != 0)
		return
	}
	pose := s.Pose(t.h)
	if pose.Space == scene.Space3D {
		switch t.pose {
		case "position":
			pose.T3.Position = v.V3
		case "rotation":
			pose.T3.Rotation = v.Q
		default:
			pose.T3.Scale = v.V3
		}
	} else {
		switch t.pose {
		case "position":
			pose.T2.Position = v.V
		case "rotation":
			pose.T2.Rotation = float32(v.F)
		default:
			pose.T2.Scale = v.V
		}
	}
	s.SetPose(t.h, pose)
}

func identity(t target, cur schema.Value) schema.Value {
	next := schema.Value{Kind: cur.Kind}
	switch {
	case t.pose == pathEnabled:
		next.I = 1
	case t.pose == "scale" && cur.Kind == schema.KindVec2:
		next.V.X, next.V.Y = 1, 1
	case t.pose == "scale":
		next.V3.X, next.V3.Y, next.V3.Z = 1, 1, 1
	case cur.Kind == schema.KindQuat:
		next.Q.W = 1
	}
	return next
}

func (d *Document) setPose(op Op, t target, raw aetext.Value, reset bool) (step, error) {
	s := d.Scene
	if t.h == s.Root() {
		return step{}, errs.New(errs.EditorProps, "the root node has no transform")
	}
	cur, fd := d.poseValue(t)
	var nv schema.Value
	var err error
	switch {
	case reset:
		nv = identity(t, cur)
	case t.lane != "":
		nv, err = setLane(cur, t.lane, raw)
	default:
		nv, err = d.read(fd, raw)
	}
	if err != nil {
		return step{}, err
	}
	if err := d.check(fd, nv, t.base); err != nil {
		return step{}, err
	}
	d.writePose(t, nv)
	ch := d.change(op.Kind, t.h)
	ch.Path, ch.Before, ch.After = t.base, d.text(fd, cur), d.text(fd, nv)
	return step{
		inv:    SetValue(idRef(s, t.h), t.base, sceneio.WriteValue(d.reg, fd, cur)),
		redo:   SetValue(idRef(s, t.h), t.base, sceneio.WriteValue(d.reg, fd, nv)),
		change: ch,
	}, nil
}
