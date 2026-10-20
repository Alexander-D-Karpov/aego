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

func Touched(h *history.History) map[Key]bool {
	out := map[Key]bool{}
	if h == nil {
		return out
	}
	h.Walk(func(c history.Command) {
		switch c := c.(type) {
		case *commands.Mutation:
			for _, ch := range c.Changes() {
				switch {
				case ch.Op != mutate.KindSet && ch.Op != mutate.KindReset:
				case ch.Type != 0:
					out[Key{Node: ch.NodeID, Type: ch.Type, Field: ch.Field}] = true
				case strings.HasPrefix(ch.Path, "transform."):
					out[Key{Node: ch.NodeID}] = true
				}
			}
		case *commands.SetTransform:
			out[Key{Node: c.Node}] = true
		case *commands.SetTransform3D:
			out[Key{Node: c.Node}] = true
		}
	})
	return out
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
