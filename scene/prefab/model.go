package prefab

import (
	"maps"
	"slices"
	"strings"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/schema"
)

type PrefabID = ids.AssetID

const (
	OwnerNode      = "node"
	OwnerTransform = "transform"
	ScriptPrefix   = "script/"
	FieldName      = "name"
	FieldEnabled   = "enabled"
	FieldLayers    = "layers"
	FieldPosition  = "position"
	FieldRotation  = "rotation"
	FieldScale     = "scale"
	FieldActive    = "#enabled"
)

type CompKind uint8

const (
	CompNode CompKind = iota
	CompTransform
	CompComponent
	CompScript
	CompUnknown
)

func (k CompKind) Removable() bool {
	return k == CompComponent || k == CompScript || k == CompUnknown
}

type Field struct {
	Name  string
	Value schema.Value
}

type Comp struct {
	Owner  string
	Kind   CompKind
	Fields []Field
	Raw    any
}

func ScriptOwner(full string) string {
	return ScriptPrefix + full
}

func (c *Comp) Get(name string) (schema.Value, bool) {
	for i := range c.Fields {
		if c.Fields[i].Name == name {
			return c.Fields[i].Value, true
		}
	}
	return schema.Value{}, false
}

func (c *Comp) Set(name string, v schema.Value) {
	for i := range c.Fields {
		if c.Fields[i].Name == name {
			c.Fields[i].Value = v
			return
		}
	}
	c.Fields = append(c.Fields, Field{Name: name, Value: v})
}

func (c Comp) Clone() Comp {
	c.Fields = slices.Clone(c.Fields)
	for i := range c.Fields {
		c.Fields[i].Value = c.Fields[i].Value.Clone()
	}
	return c
}

func CloneComps(list []Comp) []Comp {
	if list == nil {
		return nil
	}
	out := make([]Comp, len(list))
	for i := range list {
		out[i] = list[i].Clone()
	}
	return out
}

type PropKey struct {
	Owner string
	Field string
}

func (p PropKey) String() string {
	return p.Owner + "." + p.Field
}

func ParsePropKey(s string) (PropKey, bool) {
	i := strings.LastIndexByte(s, '.')
	if i <= 0 || i == len(s)-1 {
		return PropKey{}, false
	}
	return PropKey{Owner: s[:i], Field: s[i+1:]}, true
}

type Instance struct {
	Prefab    PrefabID
	Overrides Overrides
	IDs       map[Key]ids.NodeID
	Raw       any
}

func (i *Instance) Clone() *Instance {
	if i == nil {
		return nil
	}
	out := &Instance{Prefab: i.Prefab, Overrides: i.Overrides.Clone(), Raw: i.Raw}
	if i.IDs != nil {
		out.IDs = maps.Clone(i.IDs)
	}
	return out
}

type Node struct {
	Local    ids.NodeID
	Space3D  bool
	Comps    []Comp
	Children []*Node
	Instance *Instance
	Key      Key
	Origin   *Instance
	Missing  bool
}

func Transform(space3D bool) Comp {
	if space3D {
		return Comp{Owner: OwnerTransform, Kind: CompTransform, Fields: []Field{
			{Name: FieldPosition, Value: schema.Vec3(mathx.Vec3{})},
			{Name: FieldRotation, Value: schema.Quat(mathx.QuatIdentity())},
			{Name: FieldScale, Value: schema.Vec3(mathx.Vec3{X: 1, Y: 1, Z: 1})},
		}}
	}
	return Comp{Owner: OwnerTransform, Kind: CompTransform, Fields: []Field{
		{Name: FieldPosition, Value: schema.Vec2(mathx.Vec2{})},
		{Name: FieldRotation, Value: schema.Float32(0)},
		{Name: FieldScale, Value: schema.Vec2(mathx.Vec2{X: 1, Y: 1})},
	}}
}

func NewNode(local ids.NodeID, name string, space3D bool) *Node {
	return &Node{
		Local:   local,
		Space3D: space3D,
		Key:     LocalKey(local),
		Comps: []Comp{
			{Owner: OwnerNode, Kind: CompNode, Fields: []Field{
				{Name: FieldName, Value: schema.String(name)},
				{Name: FieldEnabled, Value: schema.Bool(true)},
				{Name: FieldLayers, Value: schema.Int64(1)},
			}},
			Transform(space3D),
		},
	}
}

func (n *Node) Comp(owner string) *Comp {
	for i := range n.Comps {
		if n.Comps[i].Owner == owner {
			return &n.Comps[i]
		}
	}
	return nil
}

func (n *Node) RemoveComp(owner string) bool {
	for i := range n.Comps {
		if n.Comps[i].Owner == owner {
			n.Comps = slices.Delete(n.Comps, i, i+1)
			return true
		}
	}
	return false
}

func (n *Node) Get(p PropKey) (schema.Value, bool) {
	if n == nil {
		return schema.Value{}, false
	}
	c := n.Comp(p.Owner)
	if c == nil {
		return schema.Value{}, false
	}
	return c.Get(p.Field)
}

func (n *Node) Name() string {
	v, _ := n.Get(PropKey{Owner: OwnerNode, Field: FieldName})
	return v.S
}

func (n *Node) Clone() *Node {
	if n == nil {
		return nil
	}
	out := *n
	out.Comps = CloneComps(n.Comps)
	out.Instance = n.Instance.Clone()
	out.Children = nil
	for _, c := range n.Children {
		out.Children = append(out.Children, c.Clone())
	}
	return &out
}

func (n *Node) Walk(fn func(n, parent *Node)) {
	n.walk(nil, fn)
}

func (n *Node) walk(parent *Node, fn func(n, parent *Node)) {
	fn(n, parent)
	for _, c := range n.Children {
		c.walk(n, fn)
	}
}

func (n *Node) Find(k Key) *Node {
	var out *Node
	n.Walk(func(c, _ *Node) {
		if out == nil && c.Key == k {
			out = c
		}
	})
	return out
}

func Index(t *Node) (map[Key]*Node, map[Key]Key) {
	nodes := map[Key]*Node{}
	parents := map[Key]Key{}
	t.Walk(func(n, p *Node) {
		nodes[n.Key] = n
		if p != nil {
			parents[n.Key] = p.Key
		}
	})
	return nodes, parents
}

func (n *Node) removeChild(k Key) {
	n.Children = slices.DeleteFunc(n.Children, func(c *Node) bool { return c.Key == k })
}

func (n *Node) reorder(keys []Key) {
	out := make([]*Node, 0, len(n.Children))
	used := make(map[Key]bool, len(keys))
	for _, k := range keys {
		if used[k] {
			continue
		}
		for _, c := range n.Children {
			if c.Key == k {
				out = append(out, c)
				used[k] = true
				break
			}
		}
	}
	for _, c := range n.Children {
		if !used[c.Key] {
			out = append(out, c)
		}
	}
	n.Children = out
}
