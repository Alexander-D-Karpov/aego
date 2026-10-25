package anim

import (
	"strconv"

	"aego/core/curve"
	"aego/core/errs"
	"aego/core/ids"
	"aego/format/aetext"
	"aego/schema"
)

const (
	Head          = "anim"
	TextVersion   = 1
	SpriteProp    = "SpriteRenderer.sprite"
	FlipProp      = "SpriteRenderer.flipX"
	defaultFrames = 12
)

var (
	loopNames = []string{"once", "loop", "pingPong"}
	authNames = []string{"presentation", "simulation"}
	endNames  = []string{"hold", "restore"}
	modeNames = []string{"gameplay", "presentation", "preview"}
)

func nameOf(list []string, i uint8) string {
	if int(i) < len(list) {
		return list[i]
	}
	return list[0]
}

func indexOf(list []string, s string) (uint8, bool) {
	for i, n := range list {
		if n == s {
			return uint8(i), true
		}
	}
	return 0, false
}

func (l LoopMode) String() string  { return nameOf(loopNames, uint8(l)) }
func (a Authority) String() string { return nameOf(authNames, uint8(a)) }
func (e OnEnd) String() string     { return nameOf(endNames, uint8(e)) }
func (m Mode) String() string      { return nameOf(modeNames, uint8(m)) }

func ParseLoop(s string) (LoopMode, bool) {
	i, ok := indexOf(loopNames, s)
	return LoopMode(i), ok
}

func ParseMode(s string) (Mode, bool) {
	i, ok := indexOf(modeNames, s)
	return Mode(i), ok
}

func ParseKind(s string) (schema.Kind, bool) {
	for k := schema.KindBool; k <= schema.KindCurve; k++ {
		if Supported(k) && k.String() == s {
			return k, true
		}
	}
	return schema.KindInvalid, false
}

func textErr(n *aetext.Node, msg string) error {
	if n != nil && n.Pos.Line > 0 {
		msg = "line " + strconv.Itoa(n.Pos.Line) + ": " + msg
	}
	return errs.New(errs.AssetBadArtifact, msg)
}

func word(v aetext.Value) string {
	if v.Kind == aetext.VIdent {
		return v.Ident
	}
	return v.Str
}

func attrName(n *aetext.Node, attr string, list []string) (uint8, error) {
	v, ok := n.Attr(attr)
	if !ok {
		return 0, nil
	}
	i, known := indexOf(list, word(v))
	if !known {
		return 0, textErr(n, "unknown "+attr+" "+word(v))
	}
	return i, nil
}

func Decode(n *aetext.Node) (*Clip, error) {
	if n == nil || n.Head != Head {
		return nil, textErr(n, "the document is not an animation clip")
	}
	if err := strict(n, clipShape); err != nil {
		return nil, err
	}
	if v, ok := n.Attr("version"); ok && v.AsInt() > TextVersion {
		return nil, textErr(n, "the clip is written by a newer engine, version "+strconv.FormatInt(v.AsInt(), 10))
	}
	lv, ok := n.Attr("length")
	if !ok {
		return nil, textErr(n, "the clip needs a length")
	}
	c := &Clip{Length: float32(lv.AsFloat())}
	if v, ok := n.Attr("fps"); ok {
		c.FPS = float32(v.AsFloat())
	}
	if v, ok := n.Attr("loop"); ok {
		l, known := ParseLoop(word(v))
		if !known {
			return nil, textErr(n, "unknown loop "+word(v))
		}
		c.Loop = l
	}
	for _, b := range n.Blocks {
		switch b.Head {
		case "track":
			t, err := decodeTrack(b)
			if err != nil {
				return nil, err
			}
			c.Tracks = append(c.Tracks, t)
		case "frames":
			ts, err := decodeFrames(b)
			if err != nil {
				return nil, err
			}
			c.Tracks = append(c.Tracks, ts...)
		case "event":
			e, err := decodeEvent(b)
			if err != nil {
				return nil, err
			}
			c.Events = append(c.Events, e)
		default:
			return nil, textErr(b, "unknown block "+b.Head)
		}
	}
	c.Fix()
	return c, nil
}

func decodeTrack(b *aetext.Node) (Track, error) {
	t := Track{Target: b.Label}
	if err := strict(b, trackShape); err != nil {
		return t, err
	}
	p, ok := b.Attr("prop")
	if !ok || word(p) == "" {
		return t, textErr(b, "the track needs prop")
	}
	t.Property = word(p)
	kv, ok := b.Attr("kind")
	if !ok {
		return t, textErr(b, "the track "+t.Property+" needs kind")
	}
	if t.Kind, ok = ParseKind(word(kv)); !ok {
		return t, textErr(b, "the track "+t.Property+" has kind "+word(kv)+", which cannot be animated")
	}
	if v, ok := b.Attr("numeric"); ok {
		t.Numeric = v.Bool
	}
	if v, ok := b.Attr("root"); ok {
		t.Root = v.Bool
	}
	a, err := attrName(b, "authority", authNames)
	if err != nil {
		return t, err
	}
	e, err := attrName(b, "onEnd", endNames)
	if err != nil {
		return t, err
	}
	t.Authority, t.OnEnd = Authority(a), OnEnd(e)
	for _, kb := range b.Blocks {
		if kb.Head != "key" {
			return t, textErr(kb, "a track holds only key blocks")
		}
		k, err := decodeKey(kb, t.Kind)
		if err != nil {
			return t, err
		}
		t.Keys = append(t.Keys, k)
	}
	return t, nil
}

func decodeKey(n *aetext.Node, kind schema.Kind) (Key, error) {
	var k Key
	if err := strict(n, keyShape); err != nil {
		return k, err
	}
	tv, ok := n.Attr("t")
	if !ok {
		return k, textErr(n, "the key needs t")
	}
	k.T = float32(tv.AsFloat())
	vv, ok := n.Attr("v")
	if !ok {
		return k, textErr(n, "the key needs v")
	}
	if k.V, ok = readValue(kind, vv); !ok {
		return k, textErr(n, "the key value does not match the kind "+kind.String())
	}
	if v, ok := n.Attr("interp"); ok {
		i, known := curve.ParseInterp(word(v))
		if !known {
			return k, textErr(n, "unknown interp "+word(v))
		}
		k.Interp = i
	}
	if v, ok := n.Attr("tangent"); ok {
		tg, known := curve.ParseTangent(word(v))
		if !known {
			return k, textErr(n, "unknown tangent "+word(v))
		}
		k.Tangent = tg
	}
	if k.Tangent != curve.TangentFree || classOf(kind) != classLanes {
		return k, nil
	}
	in, okIn := n.Attr("in")
	out, okOut := n.Attr("out")
	if !okIn || !okOut {
		return k, textErr(n, "a free tangent needs in and out")
	}
	lanes := lanesOf(kind)
	if k.In, ok = readLanes(in, lanes); !ok {
		return k, textErr(n, "in does not match the kind "+kind.String())
	}
	if k.Out, ok = readLanes(out, lanes); !ok {
		return k, textErr(n, "out does not match the kind "+kind.String())
	}
	return k, nil
}

func readLanes(v aetext.Value, n int) ([4]float32, bool) {
	var l [4]float32
	if n == 1 {
		if v.Kind != aetext.VInt && v.Kind != aetext.VFloat {
			return l, false
		}
		l[0] = float32(v.AsFloat())
		return l, true
	}
	if v.Kind != aetext.VList || len(v.List) != n {
		return l, false
	}
	for i, x := range v.List {
		if x.Kind != aetext.VInt && x.Kind != aetext.VFloat {
			return l, false
		}
		l[i] = float32(x.AsFloat())
	}
	return l, true
}

func readValue(kind schema.Kind, v aetext.Value) (Value, bool) {
	switch classOf(kind) {
	case classLanes, classQuat:
		if kind == schema.KindColor && v.Kind == aetext.VColor {
			c := v.Color
			return Lanes([4]float32{float32(c>>24) / 255, float32(c>>16&0xff) / 255, float32(c>>8&0xff) / 255, float32(c&0xff) / 255}), true
		}
		l, ok := readLanes(v, lanesOf(kind))
		return Lanes(l), ok
	case classInt, classDiscrete:
		switch v.Kind {
		case aetext.VInt:
			return Int(v.Int), true
		case aetext.VBool:
			return Bool(v.Bool), kind == schema.KindBool
		}
	case classAsset:
		if v.Kind != aetext.VAsset {
			return Value{}, false
		}
		id, err := ids.ParseAssetID(v.Str)
		return Asset(id), err == nil
	}
	return Value{}, false
}

func decodeFrames(b *aetext.Node) ([]Track, error) {
	if err := strict(b, framesShape); err != nil {
		return nil, err
	}
	sv, ok := b.Field("sprites")
	if !ok || sv.Kind != aetext.VList || len(sv.List) == 0 {
		return nil, textErr(b, "frames needs a list of sprites")
	}
	n := len(sv.List)
	durs, err := frameDurations(b, n)
	if err != nil {
		return nil, err
	}
	var start float32
	if v, ok := b.Attr("start"); ok {
		start = float32(v.AsFloat())
	}
	a, err := attrName(b, "authority", authNames)
	if err != nil {
		return nil, err
	}
	sprite := Track{Target: b.Label, Property: SpriteProp, Kind: schema.KindAssetRef, Authority: Authority(a)}
	if v, ok := b.Attr("prop"); ok {
		sprite.Property = word(v)
	}
	flip := Track{Target: b.Label, Property: FlipProp, Kind: schema.KindBool, Authority: Authority(a)}
	if v, ok := b.Attr("flipProp"); ok {
		flip.Property = word(v)
	}
	var flips []aetext.Value
	if v, ok := b.Field("flipX"); ok {
		if v.Kind != aetext.VList || len(v.List) != n {
			return nil, textErr(b, "flipX needs one value per sprite")
		}
		flips = v.List
	}
	t := start
	for i, s := range sv.List {
		val, ok := readValue(schema.KindAssetRef, s)
		if !ok {
			return nil, textErr(b, "sprite "+strconv.Itoa(i)+" is not an asset reference")
		}
		sprite.Keys = append(sprite.Keys, Key{T: t, V: val, Interp: curve.InterpStep})
		if flips != nil {
			if flips[i].Kind != aetext.VBool {
				return nil, textErr(b, "flipX holds true or false")
			}
			flip.Keys = append(flip.Keys, Key{T: t, V: Bool(flips[i].Bool), Interp: curve.InterpStep})
		}
		t += durs[i]
	}
	if flips == nil {
		return []Track{sprite}, nil
	}
	return []Track{sprite, flip}, nil
}

func frameDurations(b *aetext.Node, n int) ([]float32, error) {
	out := make([]float32, n)
	if v, ok := b.Field("durations"); ok {
		if v.Kind != aetext.VList || len(v.List) != n {
			return nil, textErr(b, "durations needs one value per sprite")
		}
		for i, d := range v.List {
			if out[i] = float32(d.AsFloat()); !(out[i] > 0) {
				return nil, textErr(b, "frame durations must be positive")
			}
		}
		return out, nil
	}
	fps := float32(defaultFrames)
	if v, ok := b.Attr("fps"); ok {
		fps = float32(v.AsFloat())
	}
	if !(fps > 0) {
		return nil, textErr(b, "frames fps must be positive")
	}
	for i := range out {
		out[i] = 1 / fps
	}
	return out, nil
}

func decodeEvent(b *aetext.Node) (Event, error) {
	e := Event{ID: -1}
	if err := strict(b, eventShape); err != nil {
		return e, err
	}
	tv, ok := b.Attr("t")
	if !ok {
		return e, textErr(b, "the event needs t")
	}
	e.T = float32(tv.AsFloat())
	nv, ok := b.Attr("name")
	if !ok || word(nv) == "" {
		return e, textErr(b, "the event needs a name")
	}
	e.Name = word(nv)
	if v, ok := b.Attr("payload"); ok {
		switch v.Kind {
		case aetext.VBool:
			if v.Bool {
				e.Payload = 1
			}
		case aetext.VInt, aetext.VFloat:
			e.Payload = float32(v.AsFloat())
		default:
			return e, textErr(b, "the event payload is a number or true/false")
		}
	}
	m, err := attrName(b, "mode", modeNames)
	if err != nil {
		return e, err
	}
	e.Mode = Mode(m)
	return e, nil
}

func field(name string, v aetext.Value) aetext.Field {
	return aetext.Field{Name: name, Value: v}
}

func Encode(c *Clip) *aetext.Node {
	n := &aetext.Node{Head: Head, Attrs: []aetext.Field{
		field("version", aetext.Int(TextVersion)),
		field("length", aetext.Float32(c.Length)),
	}}
	if c.FPS > 0 {
		n.Attrs = append(n.Attrs, field("fps", aetext.Float32(c.FPS)))
	}
	if c.Loop != LoopOnce {
		n.Attrs = append(n.Attrs, field("loop", aetext.Ident(c.Loop.String())))
	}
	for i := range c.Tracks {
		n.Blocks = append(n.Blocks, encodeTrack(&c.Tracks[i]))
	}
	for i := range c.Events {
		n.Blocks = append(n.Blocks, encodeEvent(&c.Events[i]))
	}
	return n
}

func encodeTrack(t *Track) *aetext.Node {
	n := &aetext.Node{Head: "track", Label: t.Target, Attrs: []aetext.Field{
		field("prop", aetext.String(t.Property)),
		field("kind", aetext.Ident(t.Kind.String())),
	}}
	if t.Numeric {
		n.Attrs = append(n.Attrs, field("numeric", aetext.Bool(true)))
	}
	if t.Root {
		n.Attrs = append(n.Attrs, field("root", aetext.Bool(true)))
	}
	if t.Authority != AuthPresentation {
		n.Attrs = append(n.Attrs, field("authority", aetext.Ident(t.Authority.String())))
	}
	if t.OnEnd != EndHold {
		n.Attrs = append(n.Attrs, field("onEnd", aetext.Ident(t.OnEnd.String())))
	}
	for i := range t.Keys {
		n.Blocks = append(n.Blocks, encodeKey(&t.Keys[i], t.Kind))
	}
	return n
}

func writeLanes(l [4]float32, n int) aetext.Value {
	if n == 1 {
		return aetext.Float32(l[0])
	}
	items := make([]aetext.Value, n)
	for i := range items {
		items[i] = aetext.Float32(l[i])
	}
	return aetext.List(items...)
}

func writeValue(kind schema.Kind, v Value) aetext.Value {
	switch classOf(kind) {
	case classLanes, classQuat:
		return writeLanes(v.L, lanesOf(kind))
	case classAsset:
		return aetext.AssetRef(v.A.String())
	}
	if kind == schema.KindBool {
		return aetext.Bool(v.I != 0)
	}
	return aetext.Int(v.I)
}

func encodeKey(k *Key, kind schema.Kind) *aetext.Node {
	n := &aetext.Node{Head: "key", Attrs: []aetext.Field{
		field("t", aetext.Float32(k.T)),
		field("v", writeValue(kind, k.V)),
	}}
	if k.Interp != curve.InterpBezier {
		n.Attrs = append(n.Attrs, field("interp", aetext.Ident(k.Interp.String())))
	}
	if k.Tangent != curve.TangentAuto {
		n.Attrs = append(n.Attrs, field("tangent", aetext.Ident(k.Tangent.String())))
	}
	if k.Tangent == curve.TangentFree && classOf(kind) == classLanes {
		lanes := lanesOf(kind)
		n.Attrs = append(n.Attrs, field("in", writeLanes(k.In, lanes)), field("out", writeLanes(k.Out, lanes)))
	}
	return n
}

func encodeEvent(e *Event) *aetext.Node {
	n := &aetext.Node{Head: "event", Attrs: []aetext.Field{
		field("t", aetext.Float32(e.T)),
		field("name", aetext.String(e.Name)),
	}}
	if e.Payload != 0 {
		n.Attrs = append(n.Attrs, field("payload", aetext.Float32(e.Payload)))
	}
	if e.Mode != EventGameplay {
		n.Attrs = append(n.Attrs, field("mode", aetext.Ident(e.Mode.String())))
	}
	return n
}
