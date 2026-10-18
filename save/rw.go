package save

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/scene"
)

type Refs interface {
	RefValue(h scene.NodeHandle) (aetext.Value, bool)
}

type Resolver interface {
	ResolveRef(v aetext.Value) (scene.NodeHandle, bool)
}

type Writer struct {
	n    *aetext.Node
	refs Refs
}

func NewWriter(n *aetext.Node, refs Refs) *Writer {
	return &Writer{n: n, refs: refs}
}

func (w *Writer) Node() *aetext.Node {
	return w.n
}

func (w *Writer) Empty() bool {
	return len(w.n.Fields) == 0 && len(w.n.Blocks) == 0
}

func (w *Writer) Set(name string, v aetext.Value) {
	w.n.SetField(name, v)
}

func (w *Writer) Bool(name string, v bool) {
	w.Set(name, aetext.Bool(v))
}

func (w *Writer) Int(name string, v int64) {
	w.Set(name, aetext.Int(v))
}

func (w *Writer) Float(name string, v float64) {
	w.Set(name, aetext.Float64(v))
}

func (w *Writer) String(name, v string) {
	w.Set(name, aetext.String(v))
}

func floats(vs ...float32) aetext.Value {
	out := make([]aetext.Value, len(vs))
	for i, f := range vs {
		out[i] = aetext.Float32(f)
	}
	return aetext.List(out...)
}

func (w *Writer) Vec2(name string, v mathx.Vec2) {
	w.Set(name, floats(v.X, v.Y))
}

func (w *Writer) Vec3(name string, v mathx.Vec3) {
	w.Set(name, floats(v.X, v.Y, v.Z))
}

func (w *Writer) Vec4(name string, v mathx.Vec4) {
	w.Set(name, floats(v.X, v.Y, v.Z, v.W))
}

func (w *Writer) Color(name string, c mathx.Color) {
	w.Set(name, floats(c.R, c.G, c.B, c.A))
}

func (w *Writer) Asset(name string, id ids.AssetID) {
	w.Set(name, aetext.AssetRef(id.String()))
}

func (w *Writer) Ref(name string, h scene.NodeHandle) bool {
	if h.IsZero() {
		w.Set(name, aetext.NodeRef(ids.NodeID{}.String()))
		return true
	}
	if w.refs == nil {
		return false
	}
	v, ok := w.refs.RefValue(h)
	if ok {
		w.Set(name, v)
	}
	return ok
}

func (w *Writer) RefValue(h scene.NodeHandle) (aetext.Value, bool) {
	if h.IsZero() {
		return aetext.NodeRef(ids.NodeID{}.String()), true
	}
	if w.refs == nil {
		return aetext.Value{}, false
	}
	return w.refs.RefValue(h)
}

func (w *Writer) Block(head, label string) *Writer {
	b := &aetext.Node{Head: head, Label: label}
	w.n.Blocks = append(w.n.Blocks, b)
	return &Writer{n: b, refs: w.refs}
}

type Reader struct {
	n   *aetext.Node
	res Resolver
}

func NewReader(n *aetext.Node, res Resolver) *Reader {
	if n == nil {
		return nil
	}
	return &Reader{n: n, res: res}
}

func (r *Reader) Node() *aetext.Node {
	if r == nil {
		return nil
	}
	return r.n
}

func (r *Reader) Label() string {
	if r == nil {
		return ""
	}
	return r.n.Label
}

func (r *Reader) Value(name string) (aetext.Value, bool) {
	if r == nil {
		return aetext.Value{}, false
	}
	return r.n.Field(name)
}

func (r *Reader) Has(name string) bool {
	_, ok := r.Value(name)
	return ok
}

func (r *Reader) Bool(name string, def bool) bool {
	if v, ok := r.Value(name); ok && v.Kind == aetext.VBool {
		return v.Bool
	}
	return def
}

func (r *Reader) Int(name string, def int64) int64 {
	if v, ok := r.Value(name); ok && (v.Kind == aetext.VInt || v.Kind == aetext.VFloat) {
		return v.AsInt()
	}
	return def
}

func (r *Reader) Float(name string, def float64) float64 {
	if v, ok := r.Value(name); ok && (v.Kind == aetext.VInt || v.Kind == aetext.VFloat) {
		return v.AsFloat()
	}
	return def
}

func (r *Reader) String(name, def string) string {
	if v, ok := r.Value(name); ok && v.Kind == aetext.VString {
		return v.Str
	}
	return def
}

func lanes(v aetext.Value, n int) ([4]float32, bool) {
	var out [4]float32
	if v.Kind != aetext.VList || len(v.List) != n {
		return out, false
	}
	for i := 0; i < n; i++ {
		out[i] = float32(v.List[i].AsFloat())
	}
	return out, true
}

func (r *Reader) Vec2(name string, def mathx.Vec2) mathx.Vec2 {
	if v, ok := r.Value(name); ok {
		if l, ok := lanes(v, 2); ok {
			return mathx.Vec2{X: l[0], Y: l[1]}
		}
	}
	return def
}

func (r *Reader) Vec3(name string, def mathx.Vec3) mathx.Vec3 {
	if v, ok := r.Value(name); ok {
		if l, ok := lanes(v, 3); ok {
			return mathx.Vec3{X: l[0], Y: l[1], Z: l[2]}
		}
	}
	return def
}

func (r *Reader) Vec4(name string, def mathx.Vec4) mathx.Vec4 {
	if v, ok := r.Value(name); ok {
		if l, ok := lanes(v, 4); ok {
			return mathx.Vec4{X: l[0], Y: l[1], Z: l[2], W: l[3]}
		}
	}
	return def
}

func (r *Reader) Color(name string, def mathx.Color) mathx.Color {
	if v, ok := r.Value(name); ok {
		if l, ok := lanes(v, 4); ok {
			return mathx.Color{R: l[0], G: l[1], B: l[2], A: l[3]}
		}
	}
	return def
}

func (r *Reader) Asset(name string) ids.AssetID {
	if v, ok := r.Value(name); ok && v.Kind == aetext.VAsset {
		if id, err := ids.ParseAssetID(v.Str); err == nil {
			return id
		}
	}
	return ids.AssetID{}
}

func (r *Reader) Ref(name string) (scene.NodeHandle, bool) {
	v, ok := r.Value(name)
	if !ok {
		return scene.NodeHandle{}, false
	}
	return r.ResolveValue(v)
}

func (r *Reader) ResolveValue(v aetext.Value) (scene.NodeHandle, bool) {
	if r == nil || r.res == nil {
		return scene.NodeHandle{}, false
	}
	return r.res.ResolveRef(v)
}

func (r *Reader) Block(head, label string) *Reader {
	if r == nil {
		return nil
	}
	return NewReader(r.n.Block(head, label), r.res)
}

func (r *Reader) Blocks(head string) []*Reader {
	if r == nil {
		return nil
	}
	var out []*Reader
	r.n.EachBlock(head, func(b *aetext.Node) {
		out = append(out, &Reader{n: b, res: r.res})
	})
	return out
}
