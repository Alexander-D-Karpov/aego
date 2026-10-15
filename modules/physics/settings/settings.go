package settings

import (
	"math"

	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/modules/physics/world"
)

const (
	ID           = "physics"
	Version      = 1
	MaxLayers    = 32
	DefaultLayer = "Default"
	LayersList   = "layers"
)

type Rule struct {
	Layer string
	With  []string
}

type Project struct {
	Gravity              mathx.Vec2
	HasGravity           bool
	Substeps             int
	VelocityIterations   int
	RelaxIterations      int
	ContactHertz         float32
	ContactDampingRatio  float32
	ContactPushVelocity  float32
	RestitutionThreshold float32
	LinearSlop           float32
	SleepLinear          float32
	SleepAngular         float32
	SleepTime            float32
	LayerNames           []string
	Collide              []Rule
}

func Default() Project {
	return Project{}
}

func Section() module.SectionDesc[Project] {
	return module.SectionDesc[Project]{
		ID:       ID,
		Title:    "Physics",
		Order:    300,
		Version:  Version,
		Default:  Default,
		Decode:   Decode,
		Encode:   Encode,
		Validate: (*Project).Validate,
		Names: func(p *Project, list string) []string {
			if list != LayersList {
				return nil
			}
			l := p.Layers()
			return l[:]
		},
	}
}

type namedFloat struct {
	name string
	v    *float32
}

type namedInt struct {
	name string
	v    *int
}

func (p *Project) floats() []namedFloat {
	return []namedFloat{
		{"contactHertz", &p.ContactHertz},
		{"contactDampingRatio", &p.ContactDampingRatio},
		{"contactPushVelocity", &p.ContactPushVelocity},
		{"restitutionThreshold", &p.RestitutionThreshold},
		{"linearSlop", &p.LinearSlop},
		{"sleepLinear", &p.SleepLinear},
		{"sleepAngular", &p.SleepAngular},
		{"sleepTime", &p.SleepTime},
	}
}

func (p *Project) ints() []namedInt {
	return []namedInt{
		{"substeps", &p.Substeps},
		{"velocityIterations", &p.VelocityIterations},
		{"relaxIterations", &p.RelaxIterations},
	}
}

func (p *Project) Config() world.Config {
	c := world.DefaultConfig()
	if p.HasGravity {
		c.Gravity = p.Gravity
	}
	for _, f := range []struct {
		dst *int
		v   int
	}{
		{&c.Substeps, p.Substeps},
		{&c.VelocityIterations, p.VelocityIterations},
		{&c.RelaxIterations, p.RelaxIterations},
	} {
		if f.v > 0 {
			*f.dst = f.v
		}
	}
	for _, f := range []struct {
		dst *float32
		v   float32
	}{
		{&c.ContactHertz, p.ContactHertz},
		{&c.ContactDampingRatio, p.ContactDampingRatio},
		{&c.ContactPushVelocity, p.ContactPushVelocity},
		{&c.RestitutionThreshold, p.RestitutionThreshold},
		{&c.LinearSlop, p.LinearSlop},
		{&c.SleepLinear, p.SleepLinear},
		{&c.SleepAngular, p.SleepAngular},
		{&c.SleepTime, p.SleepTime},
	} {
		if f.v > 0 {
			*f.dst = f.v
		}
	}
	return c
}

func ValidLayerName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return s != "true" && s != "false"
}

func (p *Project) Layers() [MaxLayers]string {
	var out [MaxLayers]string
	if len(p.LayerNames) == 0 {
		out[0] = DefaultLayer
		return out
	}
	copy(out[:], p.LayerNames)
	return out
}

func (p *Project) LayerIndex(name string) (int, bool) {
	if name == "" {
		return 0, false
	}
	for i, n := range p.Layers() {
		if n == name {
			return i, true
		}
	}
	return 0, false
}

func (p *Project) named() uint32 {
	var named uint32
	for i, n := range p.Layers() {
		if n != "" {
			named |= 1 << i
		}
	}
	return named
}

func (p *Project) Matrix() [MaxLayers]uint32 {
	var m [MaxLayers]uint32
	for i := range m {
		m[i] = ^uint32(0)
	}
	unnamed := ^p.named()
	for _, r := range p.Collide {
		a, ok := p.LayerIndex(r.Layer)
		if !ok {
			continue
		}
		row := unnamed
		for _, w := range r.With {
			if b, ok := p.LayerIndex(w); ok {
				row |= 1 << b
			}
		}
		m[a] = row
	}
	for i := 0; i < MaxLayers; i++ {
		for j := 0; j < MaxLayers; j++ {
			if (m[i]>>j)&1 == 0 {
				m[j] &^= 1 << i
			}
		}
	}
	return m
}

func (p *Project) SetMatrix(m [MaxLayers]uint32) {
	names := p.Layers()
	named := p.named()
	p.Collide = nil
	for i, n := range names {
		if n == "" || m[i]&named == named {
			continue
		}
		r := Rule{Layer: n}
		for j, w := range names {
			if w != "" && (m[i]>>j)&1 == 1 {
				r.With = append(r.With, w)
			}
		}
		p.Collide = append(p.Collide, r)
	}
}

func bad(msg string, fields ...log.Field) error {
	return errs.New(errs.ProjectBadFile, "physics: "+msg, fields...)
}

func (p *Project) Validate() error {
	if len(p.LayerNames) > MaxLayers {
		return bad("more than 32 layers")
	}
	seen := map[string]bool{}
	for i, n := range p.LayerNames {
		if n == "" {
			continue
		}
		if !ValidLayerName(n) {
			return bad("layer "+n+" is not a valid name", log.F("index", i))
		}
		if seen[n] {
			return bad("layer "+n+" is declared twice", log.F("layer", n))
		}
		seen[n] = true
	}
	keys := map[string]bool{}
	for _, r := range p.Collide {
		if _, ok := p.LayerIndex(r.Layer); !ok {
			return bad("collide names an unknown layer "+r.Layer, log.F("layer", r.Layer))
		}
		if keys[r.Layer] {
			return bad("collide lists layer "+r.Layer+" twice", log.F("layer", r.Layer))
		}
		keys[r.Layer] = true
		for _, w := range r.With {
			if _, ok := p.LayerIndex(w); !ok {
				return bad("collide rule of "+r.Layer+" names an unknown layer "+w, log.F("layer", w))
			}
		}
	}
	for _, x := range p.ints() {
		if *x.v < 0 {
			return bad(x.name+" must not be negative", log.F("field", x.name))
		}
	}
	for _, x := range p.floats() {
		if f := float64(*x.v); math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
			return bad(x.name+" must be a finite non-negative number", log.F("field", x.name))
		}
	}
	if g := p.Gravity; math.IsNaN(float64(g.X)) || math.IsNaN(float64(g.Y)) || math.IsInf(float64(g.X), 0) || math.IsInf(float64(g.Y), 0) {
		return bad("gravity is not finite")
	}
	return p.Config().Validate()
}

func nameOf(v aetext.Value) string {
	if v.Kind == aetext.VString {
		return v.Str
	}
	return v.Ident
}

func layerValue(name string) aetext.Value {
	if ValidLayerName(name) {
		return aetext.Ident(name)
	}
	return aetext.String(name)
}

func Decode(n *aetext.Node, p *Project) error {
	fl, in := p.floats(), p.ints()
fields:
	for _, f := range n.Fields {
		switch f.Name {
		case "gravity":
			if f.Value.Kind != aetext.VList || len(f.Value.List) != 2 {
				return bad("gravity must be [x, y]")
			}
			p.Gravity = mathx.Vec2{X: float32(f.Value.List[0].AsFloat()), Y: float32(f.Value.List[1].AsFloat())}
			p.HasGravity = true
			continue
		case "layers":
			if f.Value.Kind != aetext.VList {
				return bad("layers must be a list of names")
			}
			p.LayerNames = p.LayerNames[:0]
			for _, v := range f.Value.List {
				p.LayerNames = append(p.LayerNames, nameOf(v))
			}
			continue
		}
		for _, x := range fl {
			if x.name == f.Name {
				*x.v = float32(f.Value.AsFloat())
				continue fields
			}
		}
		for _, x := range in {
			if x.name == f.Name {
				*x.v = int(f.Value.AsInt())
				continue fields
			}
		}
		return bad("unknown setting "+f.Name, log.F("field", f.Name))
	}
	for _, b := range n.Blocks {
		if b.Head != "collide" {
			return bad("unknown block "+b.Head, log.F("block", b.Head))
		}
		for _, f := range b.Fields {
			if f.Value.Kind != aetext.VList {
				return bad("collide rule of "+f.Name+" must be a list of layers", log.F("layer", f.Name))
			}
			r := Rule{Layer: f.Name}
			for _, v := range f.Value.List {
				r.With = append(r.With, nameOf(v))
			}
			p.Collide = append(p.Collide, r)
		}
	}
	return nil
}

func Encode(p *Project, n *aetext.Node) {
	if p.HasGravity {
		n.Fields = append(n.Fields, aetext.Field{Name: "gravity", Value: aetext.List(aetext.Float32(p.Gravity.X), aetext.Float32(p.Gravity.Y))})
	}
	for _, x := range p.ints() {
		if *x.v != 0 {
			n.Fields = append(n.Fields, aetext.Field{Name: x.name, Value: aetext.Int(int64(*x.v))})
		}
	}
	for _, x := range p.floats() {
		if *x.v != 0 {
			n.Fields = append(n.Fields, aetext.Field{Name: x.name, Value: aetext.Float32(*x.v)})
		}
	}
	if len(p.LayerNames) > 0 && !(len(p.LayerNames) == 1 && p.LayerNames[0] == DefaultLayer) {
		list := aetext.Value{Kind: aetext.VList}
		for _, name := range p.LayerNames {
			list.List = append(list.List, layerValue(name))
		}
		n.Fields = append(n.Fields, aetext.Field{Name: "layers", Value: list})
	}
	if len(p.Collide) > 0 {
		c := &aetext.Node{Head: "collide"}
		for _, r := range p.Collide {
			list := aetext.Value{Kind: aetext.VList}
			for _, w := range r.With {
				list.List = append(list.List, layerValue(w))
			}
			c.Fields = append(c.Fields, aetext.Field{Name: r.Layer, Value: list})
		}
		n.Blocks = append(n.Blocks, c)
	}
}
