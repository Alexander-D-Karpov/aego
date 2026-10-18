package settings

import (
	"errors"
	"strconv"

	"aego/core/mathx"
	"aego/engine/problems"
	"aego/format/aetext"
	"aego/schema"
)

const (
	ProfileHead    = "profile"
	ProfileVersion = 1
	ProjectHead    = "defaults"
	versionAttr    = "version"
)

type OwnerVersion struct {
	Owner   string
	Version int
	Migrate func(n *aetext.Node, from, to int) error
}

func (s *Store) encode(e *entry, v schema.Value) aetext.Value {
	switch e.d.Kind {
	case schema.KindBool:
		return aetext.Bool(v.I != 0)
	case schema.KindInt64:
		return aetext.Int(v.I)
	case schema.KindFloat64:
		return aetext.Float64(v.F)
	case schema.KindEnum:
		if c, ok := s.choiceOf(e, v); ok && ValidPath(c.Name) {
			return aetext.Ident(c.Name)
		}
		return aetext.Int(v.I)
	case schema.KindVec2:
		return aetext.List(aetext.Float32(v.V.X), aetext.Float32(v.V.Y))
	case schema.KindColor:
		return aetext.List(aetext.Float32(v.C.R), aetext.Float32(v.C.G), aetext.Float32(v.C.B), aetext.Float32(v.C.A))
	case schema.KindList:
		out := aetext.Value{Kind: aetext.VList, List: make([]aetext.Value, len(v.List))}
		for i, x := range v.List {
			out.List[i] = aetext.String(x.S)
		}
		return out
	}
	return aetext.String(v.S)
}

func floats(a aetext.Value, n int) ([]float32, error) {
	if a.Kind != aetext.VList || len(a.List) != n {
		return nil, errors.New("expected " + strconv.Itoa(n) + " numbers")
	}
	out := make([]float32, n)
	for i, x := range a.List {
		if x.Kind != aetext.VInt && x.Kind != aetext.VFloat {
			return nil, errors.New("expected numbers")
		}
		out[i] = float32(x.AsFloat())
	}
	return out, nil
}

func (s *Store) decode(e *entry, a aetext.Value) (schema.Value, error) {
	num := a.Kind == aetext.VInt || a.Kind == aetext.VFloat
	switch e.d.Kind {
	case schema.KindBool:
		if a.Kind == aetext.VBool {
			return schema.Bool(a.Bool), nil
		}
		return schema.Value{}, errors.New("expected true or false")
	case schema.KindInt64:
		if num {
			return schema.Int64(a.AsInt()), nil
		}
		return schema.Value{}, errors.New("expected a number")
	case schema.KindFloat64:
		if num {
			return schema.Float64(a.AsFloat()), nil
		}
		return schema.Value{}, errors.New("expected a number")
	case schema.KindEnum:
		switch a.Kind {
		case aetext.VIdent:
			for _, c := range s.choices(e) {
				if c.Name == a.Ident {
					return c.Value, nil
				}
			}
			return schema.Value{}, errors.New("unknown value " + a.Ident)
		case aetext.VInt:
			return schema.Enum(a.Int), nil
		}
		return schema.Value{}, errors.New("expected a name")
	case schema.KindVec2:
		f, err := floats(a, 2)
		if err != nil {
			return schema.Value{}, err
		}
		return schema.Vec2(mathx.Vec2{X: f[0], Y: f[1]}), nil
	case schema.KindColor:
		f, err := floats(a, 4)
		if err != nil {
			return schema.Value{}, err
		}
		return schema.Color(mathx.Color{R: f[0], G: f[1], B: f[2], A: f[3]}), nil
	case schema.KindList:
		if a.Kind != aetext.VList {
			return schema.Value{}, errors.New("expected a list of strings")
		}
		out := schema.Value{Kind: schema.KindList, List: make([]schema.Value, len(a.List))}
		for i, x := range a.List {
			if x.Kind != aetext.VString {
				return schema.Value{}, errors.New("expected a list of strings")
			}
			out.List[i] = schema.String(x.Str)
		}
		return out, nil
	}
	if a.Kind != aetext.VString {
		return schema.Value{}, errors.New("expected a string")
	}
	return schema.String(a.Str), nil
}

func (s *Store) write(root *aetext.Node, user bool, pick func(Key, *entry) (schema.Value, bool)) {
	blocks := map[string]*aetext.Node{}
	get := func(name string) *aetext.Node {
		if b := blocks[name]; b != nil {
			return b
		}
		b := &aetext.Node{Head: name}
		if o := s.owners[name]; user && o.version > 1 {
			b.Attrs = []aetext.Field{{Name: versionAttr, Value: aetext.Int(int64(o.version))}}
		}
		blocks[name] = b
		return b
	}
	for i := range s.list {
		e := &s.list[i]
		if v, ok := pick(Key(i+1), e); ok {
			b := get(e.d.Owner)
			b.Fields = append(b.Fields, aetext.Field{Name: e.d.Path(), Value: s.encode(e, v)})
		}
	}
	for _, name := range s.order {
		o := s.owners[name]
		if user && o.newer != nil {
			root.Blocks = append(root.Blocks, o.newer)
			continue
		}
		extra := o.projExtra
		if user {
			extra = o.userExtra
		}
		if len(extra) > 0 {
			b := get(name)
			b.Fields = append(b.Fields, extra...)
		}
		if b := blocks[name]; b != nil {
			root.Blocks = append(root.Blocks, b)
		}
	}
}

func (s *Store) EncodeProfile() []byte {
	root := &aetext.Node{Head: ProfileHead, Attrs: []aetext.Field{{Name: versionAttr, Value: aetext.Int(ProfileVersion)}}}
	s.write(root, true, func(_ Key, e *entry) (schema.Value, bool) {
		return e.user, e.hasUser && !e.d.Flags.Has(Transient)
	})
	root.Blocks = append(root.Blocks, s.unknownUser...)
	return aetext.Write(root)
}

func (s *Store) ProjectNode(vals map[Key]schema.Value) *aetext.Node {
	root := &aetext.Node{Head: ProjectHead}
	s.write(root, false, func(k Key, _ *entry) (schema.Value, bool) {
		v, ok := vals[k]
		return v, ok
	})
	root.Blocks = append(root.Blocks, s.unknownProject...)
	if len(root.Blocks) == 0 {
		return nil
	}
	return root
}

func (s *Store) ProjectValues() map[Key]schema.Value {
	out := map[Key]schema.Value{}
	for i := range s.list {
		if e := &s.list[i]; e.hasProject {
			out[Key(i+1)] = clone(e.project)
		}
	}
	return out
}

func (s *Store) read(root *aetext.Node, user bool, name string) (map[Key]schema.Value, []*aetext.Node) {
	vals := map[Key]schema.Value{}
	var unknown []*aetext.Node
	need := Project
	if user {
		need = User
	}
	for _, b := range root.Blocks {
		o, ok := s.owners[b.Head]
		if !ok {
			unknown = append(unknown, b.Clone())
			continue
		}
		n := b
		if user {
			ver := 1
			if a, ok := b.Attr(versionAttr); ok {
				ver = int(a.AsInt())
			}
			switch {
			case ver > o.version:
				o.newer = b.Clone()
				s.report("CFG006", problems.Warning, name+": "+b.Head+" settings come from a newer version and are kept as they are", "")
				continue
			case ver < o.version:
				if o.migrate == nil {
					s.report("CFG007", problems.Warning, name+": "+b.Head+" settings of version "+strconv.Itoa(ver)+" have no migration, defaults are used", "")
					continue
				}
				n = b.Clone()
				if err := o.migrate(n, ver, o.version); err != nil {
					s.report("CFG007", problems.Warning, name+": "+b.Head+": "+err.Error(), "")
					continue
				}
			}
		}
		for _, f := range n.Fields {
			k, ok := s.byID[b.Head+"."+f.Name]
			if !ok {
				if user {
					o.userExtra = append(o.userExtra, f)
				} else {
					o.projExtra = append(o.projExtra, f)
				}
				continue
			}
			e := s.at(k)
			if !e.d.Flags.Has(need) {
				s.report("CFG002", problems.Warning, name+": "+e.d.ID+" cannot be set here", e.d.ID)
				continue
			}
			v, err := s.decode(e, f.Value)
			if err == nil {
				v, err = s.check(e, v)
			}
			if err != nil {
				s.report("CFG002", problems.Warning, name+": "+e.d.ID+": "+err.Error()+", the default is used", e.d.ID)
				continue
			}
			vals[k] = v
		}
	}
	return vals, unknown
}

func (s *Store) LoadProject(n *aetext.Node, name string) {
	if n == nil {
		return
	}
	vals, unknown := s.read(n, false, name)
	s.unknownProject = unknown
	for k, v := range vals {
		e := s.at(k)
		e.project, e.hasProject = v, true
		e.recompute()
	}
}

func (s *Store) LoadProfile(data []byte, name string) error {
	if len(data) == 0 {
		return nil
	}
	doc, err := aetext.Parse(data, name)
	if err == nil && doc.Head != ProfileHead {
		err = bad(name, "is not a settings profile")
	}
	if err != nil {
		s.report("CFG001", problems.Warning, name+": the settings profile is damaged, defaults are used", "")
		return err
	}
	if a, ok := doc.Attr(versionAttr); ok && int(a.AsInt()) > ProfileVersion {
		s.readOnly = true
		s.report("CFG006", problems.Warning, name+": the profile comes from a newer version, it is read-only", "")
		return nil
	}
	vals, unknown := s.read(doc, true, name)
	s.unknownUser = unknown
	for k, v := range vals {
		e := s.at(k)
		e.user, e.hasUser = v, true
		e.recompute()
	}
	return nil
}
