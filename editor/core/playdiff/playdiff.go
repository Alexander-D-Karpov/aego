package playdiff

import (
	"reflect"
	"strconv"
	"strings"

	"aego/core/curve"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/editor/core/history/commands"
	"aego/editor/core/props"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/scene/scriptcomp"
	"aego/schema"
)

type Mode uint8

const (
	ModeIfChanges Mode = iota
	ModeAlways
	ModeNever
)

var modeNames = [...]string{"if-changes", "always", "never"}

func (m Mode) String() string {
	return modeNames[m]
}

func ParseMode(s string) Mode {
	for i, n := range modeNames {
		if n == s {
			return Mode(i)
		}
	}
	return ModeIfChanges
}

type Kind uint8

const (
	KindField Kind = iota
	KindTransform2D
	KindTransform3D
)

type Key struct {
	Node  ids.NodeID
	Type  schema.TypeID
	Field schema.FieldID
}

type Change struct {
	Node     ids.NodeID
	Path     string
	Owner    string
	Label    string
	Kind     Kind
	Type     schema.TypeID
	Field    schema.FieldID
	Before   schema.Value
	After    schema.Value
	T2Before scene.Transform2D
	T2After  scene.Transform2D
	T3Before scene.Transform3D
	T3After  scene.Transform3D
	Checked  bool
	Icon     string
	Name     string
	Tip      string
	Gameplay bool
}

func (c *Change) Key() Key {
	if c.Kind == KindField {
		return Key{Node: c.Node, Type: c.Type, Field: c.Field}
	}
	return Key{Node: c.Node}
}

type Result struct {
	Changes []Change
	Created []string
	Removed []string
}

func (r *Result) Empty() bool {
	return len(r.Changes) == 0
}

func (r *Result) labelOf(c *Change, edit, play *document.SceneDocument) {
	s := play.Scene()
	h := s.Handle(c.Node)
	if h.IsZero() || !s.IsAlive(h) {
		s = edit.Scene()
		h = s.Handle(c.Node)
	}
	c.Icon, c.Name, c.Tip = props.NodeLabel(s, h, false)
	if c.Name == "" {
		c.Name = c.Path
	}
}

func nodeName(s *scene.Scene, id ids.NodeID) string {
	if _, name, _ := props.NodeLabel(s, s.Handle(id), false); name != "" {
		return name
	}
	return id.Short()
}

// FromHistory lists what the Play history changed (D283): the last edit of each field wins and its value comes from the command, not from the scene.
func FromHistory(edit, play *document.SceneDocument) Result {
	var r Result
	index := map[Key]int{}
	put := func(c Change) {
		k := c.Key()
		if i, ok := index[k]; ok {
			r.Changes[i] = c
			return
		}
		index[k] = len(r.Changes)
		r.Changes = append(r.Changes, c)
	}
	ps, es := play.Scene(), edit.Scene()
	reg := ps.Registry()
	nodes := map[ids.NodeID]bool{}
	var order []ids.NodeID
	touch := func(id ids.NodeID) {
		if !nodes[id] {
			nodes[id] = true
			order = append(order, id)
		}
	}
	play.History.Walk(func(cmd history.Command) {
		switch c := cmd.(type) {
		case *commands.Mutation:
			for _, ch := range c.Changes() {
				switch ch.Op {
				case mutate.KindSet, mutate.KindReset:
					if ch.Type == 0 {
						if strings.HasPrefix(ch.Path, "transform.") {
							put(transformChange(ps, ch.NodeID))
						}
						continue
					}
					td, ok := reg.ByID(ch.Type)
					if !ok {
						continue
					}
					fd, ok := td.FieldByID(ch.Field)
					if !ok || !diffable(fd) {
						continue
					}
					after, err := afterValue(reg, fd, ch.After)
					if err != nil {
						if after, err = play.Props.Get(ch.NodeID, ch.Type, ch.Field); err != nil {
							continue
						}
					}
					owner := td.DisplayName
					if owner == "" {
						owner = shortName(td.FullName)
					}
					put(Change{Node: ch.NodeID, Owner: owner, Label: props.Label(fd), Kind: KindField, Type: ch.Type, Field: ch.Field, After: after, Checked: true})
				case mutate.KindAddNode, mutate.KindInsert, mutate.KindRemoveNode:
					touch(ch.NodeID)
				}
			}
		case *commands.SetTransform:
			put(Change{Node: c.Node, Owner: "Transform", Label: "Transform", Kind: KindTransform2D, T2After: c.After, Checked: true})
		case *commands.SetTransform3D:
			put(Change{Node: c.Node, Owner: "Transform", Label: "Transform", Kind: KindTransform3D, T3After: c.After, Checked: true})
		}
	})
	for _, id := range order {
		if id.IsZero() {
			continue
		}
		inPlay := alive(ps, id)
		inEdit := alive(es, id)
		switch {
		case inPlay && !inEdit:
			r.Created = append(r.Created, nodeName(ps, id))
		case inEdit && !inPlay:
			r.Removed = append(r.Removed, nodeName(es, id))
		}
	}
	out := r.Changes[:0]
	for _, c := range r.Changes {
		if !alive(es, c.Node) {
			continue
		}
		one := []ids.NodeID{c.Node}
		switch c.Kind {
		case KindField:
			before, err := edit.Props.Get(c.Node, c.Type, c.Field)
			if err != nil || props.Equal(before, c.After) {
				continue
			}
			c.Before = before
		case KindTransform2D:
			if c.T2Before, _ = edit.Props.CommonTransform(one); c.T2Before == c.T2After {
				continue
			}
		case KindTransform3D:
			if c.T3Before, _, _ = edit.Props.CommonTransform3D(one); c.T3Before == c.T3After {
				continue
			}
		}
		c.Path = es.Path(es.Handle(c.Node))
		r.labelOf(&c, edit, play)
		out = append(out, c)
	}
	r.Changes = out
	return r
}

// Full is the whole scene comparison: the history rows first, then the other differences unchecked and marked as made by the game.
func Full(edit, play *document.SceneDocument, fromHistory Result) Result {
	full := Compute(edit, play, nil)
	have := map[Key]bool{}
	for _, c := range fromHistory.Changes {
		have[c.Key()] = true
	}
	out := append([]Change(nil), fromHistory.Changes...)
	for _, c := range full.Changes {
		if have[c.Key()] {
			continue
		}
		c.Checked, c.Gameplay = false, true
		full.labelOf(&c, edit, play)
		out = append(out, c)
	}
	full.Changes = groupByNode(out)
	return full
}

// groupByNode keeps the rows of one node together, nodes in the order they first appear.
func groupByNode(list []Change) []Change {
	var order []ids.NodeID
	by := map[ids.NodeID][]Change{}
	for _, c := range list {
		if _, ok := by[c.Node]; !ok {
			order = append(order, c.Node)
		}
		by[c.Node] = append(by[c.Node], c)
	}
	out := make([]Change, 0, len(list))
	for _, n := range order {
		out = append(out, by[n]...)
	}
	return out
}

func alive(s *scene.Scene, id ids.NodeID) bool {
	h := s.Handle(id)
	return !h.IsZero() && s.IsAlive(h)
}

func afterValue(reg *schema.Registry, fd *schema.FieldDescriptor, text string) (schema.Value, error) {
	v, err := mutate.ParseValue(text)
	if err != nil {
		return schema.Value{}, err
	}
	return sceneio.ReadValue(reg, fd, v)
}

func transformChange(s *scene.Scene, id ids.NodeID) Change {
	h := s.Handle(id)
	if s.Space(h) == scene.Space3D {
		return Change{Node: id, Owner: "Transform", Label: "Transform", Kind: KindTransform3D, T3After: s.Local3D(h), Checked: true}
	}
	return Change{Node: id, Owner: "Transform", Label: "Transform", Kind: KindTransform2D, T2After: s.Local(h), Checked: true}
}

func diffable(fd *schema.FieldDescriptor) bool {
	return fd.Flags.Has(schema.FlagEditor) && !fd.Flags.Has(schema.FlagReadOnly) && !fd.Hints.Hidden && !fd.Hints.Runtime
}

func Compute(edit, play *document.SceneDocument, touched map[Key]bool) Result {
	var r Result
	es, ps := edit.Scene(), play.Scene()
	es.UpdateTransforms()
	ps.UpdateTransforms()
	proot, eroot := ps.Root(), es.Root()
	for _, h := range ps.PreOrder() {
		if h == proot || !ps.IsAlive(h) {
			continue
		}
		id := ps.ID(h)
		eh := es.Handle(id)
		if eh.IsZero() || !es.IsAlive(eh) {
			r.Created = append(r.Created, ps.Path(h))
			continue
		}
		r.node(edit, play, id, h, eh, touched)
	}
	for _, h := range es.PreOrder() {
		if h == eroot || !es.IsAlive(h) {
			continue
		}
		if ph := ps.Handle(es.ID(h)); ph.IsZero() || !ps.IsAlive(ph) {
			r.Removed = append(r.Removed, es.Path(h))
		}
	}
	return r
}

func (r *Result) node(edit, play *document.SceneDocument, id ids.NodeID, ph, eh scene.NodeHandle, touched map[Key]bool) {
	es, ps := edit.Scene(), play.Scene()
	path := ps.Path(ph)
	one := []ids.NodeID{id}
	if es.Space(eh) == ps.Space(ph) {
		if es.Space(eh) == scene.Space3D {
			before, _, _ := edit.Props.CommonTransform3D(one)
			after, _, _ := play.Props.CommonTransform3D(one)
			if before != after {
				r.Changes = append(r.Changes, Change{Node: id, Path: path, Owner: "Transform", Label: "Transform", Kind: KindTransform3D,
					T3Before: before, T3After: after, Checked: touched[Key{Node: id}]})
			}
		} else {
			before, _ := edit.Props.CommonTransform(one)
			after, _ := play.Props.CommonTransform(one)
			if before != after {
				r.Changes = append(r.Changes, Change{Node: id, Path: path, Owner: "Transform", Label: "Transform", Kind: KindTransform2D,
					T2Before: before, T2After: after, Checked: touched[Key{Node: id}]})
			}
		}
	}
	reg := ps.Registry()
	unknown, hasUnknown := reg.ByName("engine.Unknown")
	ps.ComponentsOf(ph, func(t schema.TypeID) {
		if (hasUnknown && t == unknown.ID) || t == scriptcomp.ScriptTypeID {
			return
		}
		td, ok := reg.ByID(t)
		if !ok || td.Hidden {
			return
		}
		if _, ok := es.GetByType(eh, t); !ok {
			return
		}
		r.fields(edit, play, id, path, td, touched)
	})
	sc, ok := scriptcomp.ScriptsOf(ps, ph)
	if !ok {
		return
	}
	esc, ok := scriptcomp.ScriptsOf(es, eh)
	if !ok {
		return
	}
	for i := range sc.Attachments {
		name := sc.Attachments[i].TypeName
		if esc.Index(name) < 0 {
			continue
		}
		if td, ok := reg.ByName(name); ok && td.Kind == schema.TypeScript {
			r.fields(edit, play, id, path, td, touched)
		}
	}
}

func shortName(full string) string {
	for i := len(full) - 1; i >= 0; i-- {
		if full[i] == '.' {
			return full[i+1:]
		}
	}
	return full
}

func (r *Result) fields(edit, play *document.SceneDocument, id ids.NodeID, path string, td *schema.TypeDescriptor, touched map[Key]bool) {
	owner := td.DisplayName
	if owner == "" {
		owner = shortName(td.FullName)
	}
	for i := range td.Fields {
		fd := &td.Fields[i]
		if !diffable(fd) {
			continue
		}
		before, err := edit.Props.Get(id, td.ID, fd.ID)
		if err != nil {
			continue
		}
		after, err := play.Props.Get(id, td.ID, fd.ID)
		if err != nil || reflect.DeepEqual(before, after) {
			continue
		}
		k := Key{Node: id, Type: td.ID, Field: fd.ID}
		r.Changes = append(r.Changes, Change{Node: id, Path: path, Owner: owner, Label: props.Label(fd), Kind: KindField,
			Type: td.ID, Field: fd.ID, Before: before, After: after, Checked: touched[k]})
	}
}

func Apply(edit *document.SceneDocument, changes []Change) (int, error) {
	n := 0
	tx := edit.History.Begin("Apply Play Changes")
	for i := range changes {
		c := &changes[i]
		if !c.Checked {
			continue
		}
		one := []ids.NodeID{c.Node}
		var err error
		switch c.Kind {
		case KindField:
			err = edit.Props.Set(tx, one, c.Type, c.Field, c.After)
		case KindTransform2D:
			after := c.T2After
			err = edit.Props.EditTransform(tx, one, func(t *scene.Transform2D) { *t = after })
		case KindTransform3D:
			after := c.T3After
			err = edit.Props.EditTransform3D(tx, one, func(t *scene.Transform3D) { *t = after })
		}
		if err != nil {
			tx.Abort()
			return 0, err
		}
		n++
	}
	if n == 0 {
		tx.Abort()
		return 0, nil
	}
	tx.Commit()
	return n, nil
}

func num(f float32) string {
	return strconv.FormatFloat(float64(f), 'g', 5, 32)
}

func vec2(v mathx.Vec2) string {
	return num(v.X) + ", " + num(v.Y)
}

func vec3(v mathx.Vec3) string {
	return num(v.X) + ", " + num(v.Y) + ", " + num(v.Z)
}

func (c *Change) Text(after bool) string {
	switch c.Kind {
	case KindTransform2D:
		t := c.T2Before
		if after {
			t = c.T2After
		}
		return "pos " + vec2(t.Position) + "  rot " + num(mathx.Rad2Deg(t.Rotation)) + "°  scale " + vec2(t.Scale)
	case KindTransform3D:
		t := c.T3Before
		if after {
			t = c.T3After
		}
		return "pos " + vec3(t.Position) + "  rot " + vec3(props.EulerDeg(t.Rotation)) + "  scale " + vec3(t.Scale)
	}
	v := c.Before
	if after {
		v = c.After
	}
	return Value(v)
}

func Value(v schema.Value) string {
	switch v.Kind {
	case schema.KindBool:
		return strconv.FormatBool(v.I != 0)
	case schema.KindInt32, schema.KindInt64, schema.KindEnum:
		return strconv.FormatInt(v.I, 10)
	case schema.KindFloat32, schema.KindFloat64:
		return strconv.FormatFloat(v.F, 'g', 5, 64)
	case schema.KindString:
		return strconv.Quote(v.S)
	case schema.KindVec2:
		return vec2(v.V)
	case schema.KindVec3:
		return vec3(v.V3)
	case schema.KindVec4:
		return num(v.V4.X) + ", " + num(v.V4.Y) + ", " + num(v.V4.Z) + ", " + num(v.V4.W)
	case schema.KindQuat:
		return vec3(props.EulerDeg(v.Q)) + "°"
	case schema.KindColor:
		return num(v.C.R) + ", " + num(v.C.G) + ", " + num(v.C.B) + ", " + num(v.C.A)
	case schema.KindRect:
		return num(v.R.X) + ", " + num(v.R.Y) + ", " + num(v.R.W) + ", " + num(v.R.H)
	case schema.KindCurve:
		if v.Cv.Ease != curve.None {
			return v.Cv.Ease.String()
		}
		return strconv.Itoa(len(v.Cv.Keys)) + " keys"
	case schema.KindAssetRef:
		if v.Asset.IsZero() {
			return "None"
		}
		return v.Asset.String()
	case schema.KindNodeRef:
		if v.Node.IsZero() {
			return "None"
		}
		return v.Node.String()
	case schema.KindList:
		return "[" + strconv.Itoa(len(v.List)) + "]"
	}
	return "…"
}
