package mutate

import (
	"math"
	"slices"
	"strconv"

	"aego/anim"
	"aego/core/errs"
	"aego/core/log"
	"aego/format/aetext"
)

const (
	KindAnimKeySet       = "anim-key-set"
	KindAnimKeyRemove    = "anim-key-remove"
	KindAnimKeyMove      = "anim-key-move"
	KindAnimTrackRemove  = "anim-track-remove"
	KindAnimEventAdd     = "anim-event-add"
	KindAnimEventRemove  = "anim-event-remove"
	KindAnimEventMove    = "anim-event-move"
	KindAnimClipSet      = "anim-clip-set"
	KindAnimFramesExpand = "anim-frames-expand"
	KindAnimSplice       = "anim-splice"
	timeEps              = 1e-5
)

var animKinds = map[string]bool{
	KindAnimKeySet: true, KindAnimKeyRemove: true, KindAnimKeyMove: true, KindAnimTrackRemove: true,
	KindAnimEventAdd: true, KindAnimEventRemove: true, KindAnimEventMove: true, KindAnimClipSet: true,
	KindAnimFramesExpand: true, KindAnimSplice: true,
}

func AnimKeySet(target, prop, time string, v aetext.Value) Op {
	return Op{Kind: KindAnimKeySet, Node: target, Path: prop, Key: time, Value: v, HasValue: true}
}

func AnimKeyRemove(target, prop, time string) Op {
	return Op{Kind: KindAnimKeyRemove, Node: target, Path: prop, Key: time}
}

func AnimKeyMove(target, prop, from, to string) Op {
	return Op{Kind: KindAnimKeyMove, Node: target, Path: prop, Key: from, Name: to}
}

func AnimTrackRemove(target, prop string) Op {
	return Op{Kind: KindAnimTrackRemove, Node: target, Path: prop}
}

func AnimEventAdd(time, name string) Op {
	return Op{Kind: KindAnimEventAdd, Key: time, Name: name}
}

func AnimEventRemove(i int) Op {
	return Op{Kind: KindAnimEventRemove, Index: i, HasIndex: true}
}

func AnimEventMove(i int, time string) Op {
	return Op{Kind: KindAnimEventMove, Index: i, HasIndex: true, Key: time}
}

func AnimClipSet(attr string, v aetext.Value) Op {
	return Op{Kind: KindAnimClipSet, Path: attr, Value: v, HasValue: true}
}

func AnimClipClear(attr string) Op {
	return Op{Kind: KindAnimClipSet, Path: attr}
}

func AnimFramesExpand(target string) Op {
	return Op{Kind: KindAnimFramesExpand, Node: target}
}

type AnimDocument struct {
	Name string
	b    blocks
}

func LoadAnim(src []byte, name string) (*AnimDocument, error) {
	root, err := aetext.Parse(src, name)
	if err != nil {
		return nil, err
	}
	if _, err := anim.Decode(root); err != nil {
		return nil, err
	}
	return &AnimDocument{Name: name, b: blocks{root: root, splice: KindAnimSplice}}, nil
}

func (d *AnimDocument) Root() *aetext.Node {
	return d.b.root
}

func (d *AnimDocument) Bytes() []byte {
	return aetext.Write(d.b.root)
}

func (d *AnimDocument) Clip() (*anim.Clip, error) {
	return anim.Decode(d.b.root)
}

func (d *AnimDocument) Apply(tx Tx) (*Result, error) {
	return runChecked(d, tx, false, d.Problems)
}

func (d *AnimDocument) DryRun(tx Tx) (*Result, error) {
	return runChecked(d, tx, true, d.Problems)
}

func (d *AnimDocument) Problems() []Problem {
	c, err := anim.Decode(d.b.root)
	if err != nil {
		return []Problem{{Error: true, Message: err.Error()}}
	}
	var out []Problem
	for _, p := range c.Check(nil) {
		out = append(out, Problem{Error: true, Message: clipProblem(c, p)})
	}
	return out
}

func trackLabel(target, prop string) string {
	if target == "" {
		return prop
	}
	return target + ": " + prop
}

func clipProblem(c *anim.Clip, p anim.Problem) string {
	track := func() string {
		t := &c.Tracks[p.Track]
		return trackLabel(t.Target, t.Property)
	}
	switch p.Kind {
	case anim.ProblemLength:
		return "ANM002: the clip length must be positive"
	case anim.ProblemRoot:
		return "ANM001: track " + track() + ": root motion needs the node's own transform.position with kind vec2"
	case anim.ProblemBadKind:
		return "ANM001: track " + track() + " has a kind that cannot be animated"
	case anim.ProblemEmpty:
		return "ANM001: track " + track() + " has no keys"
	case anim.ProblemKeyRange:
		return "ANM002: track " + track() + " has a key at " + strconv.FormatFloat(float64(p.T), 'g', -1, 32) + ", outside the clip"
	case anim.ProblemEventRange:
		return "ANM002: event " + c.Events[p.Event].Name + " is outside the clip"
	}
	return "ANM003: an event has no name"
}

func (d *AnimDocument) do(op Op, _ *Result) (step, error) {
	backup := d.b.root.Clone()
	st, err := d.apply(op)
	if err == nil {
		_, err = anim.Decode(d.b.root)
	}
	if err != nil {
		d.b.root = backup
		return step{}, err
	}
	return st, nil
}

func (d *AnimDocument) apply(op Op) (step, error) {
	switch op.Kind {
	case KindAnimKeySet:
		return d.keySet(op)
	case KindAnimKeyRemove:
		return d.keyRemove(op)
	case KindAnimKeyMove:
		return d.keyMove(op)
	case KindAnimTrackRemove:
		i, err := d.trackOf(op.Node, op.Path)
		if err != nil {
			return step{}, err
		}
		return d.b.replace(i, 1, nil, Change{Op: op.Kind, Node: op.Node, Path: op.Path})
	case KindAnimEventAdd:
		return d.eventAdd(op)
	case KindAnimEventRemove, KindAnimEventMove:
		return d.eventEdit(op)
	case KindAnimClipSet:
		return d.clipSet(op)
	case KindAnimFramesExpand:
		return d.framesExpand(op)
	case KindAnimSplice:
		return d.b.applySplice(op)
	}
	return step{}, errs.New(errs.EditorCommand, op.Kind+" does not apply to animation clips")
}

func keyTime(s string) (float32, error) {
	f, err := strconv.ParseFloat(s, 32)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, errs.New(errs.EditorProps, "a time is a number of seconds", log.F("time", s))
	}
	return float32(f), nil
}

func timeOf(n *aetext.Node) float32 {
	v, _ := n.Attr("t")
	return float32(v.AsFloat())
}

func sameTime(a, b float32) bool {
	return math.Abs(float64(a-b)) <= timeEps
}

func keyAt(tr *aetext.Node, t float32) int {
	for i, k := range tr.Blocks {
		if k.Head == "key" && sameTime(timeOf(k), t) {
			return i
		}
	}
	return -1
}

func insertKey(tr, k *aetext.Node) {
	t := timeOf(k)
	at := len(tr.Blocks)
	for i, b := range tr.Blocks {
		if b.Head == "key" && timeOf(b) > t {
			at = i
			break
		}
	}
	tr.Blocks = slices.Insert(tr.Blocks, at, k)
}

func newKey(t float32, v aetext.Value, extra, base *aetext.Node) *aetext.Node {
	k := &aetext.Node{Head: "key"}
	if base != nil {
		k = base.Clone()
	}
	setAttr(k, "t", aetext.Float32(t))
	setAttr(k, "v", v.Clone())
	if extra != nil {
		for _, f := range extra.Attrs {
			if f.Name != "t" && f.Name != "v" {
				setAttr(k, f.Name, f.Value.Clone())
			}
		}
	}
	return k
}

func (d *AnimDocument) track(target, prop string) int {
	for i, b := range d.b.root.Blocks {
		if b.Head != "track" || b.Label != target {
			continue
		}
		if v, ok := b.Attr("prop"); ok && wordOf(v) == prop {
			return i
		}
	}
	return -1
}

func (d *AnimDocument) framesOwn(target, prop string) error {
	for _, b := range d.b.root.Blocks {
		if b.Head != "frames" || b.Label != target {
			continue
		}
		sprite, flip := anim.SpriteProp, anim.FlipProp
		if v, ok := b.Attr("prop"); ok {
			sprite = wordOf(v)
		}
		if v, ok := b.Attr("flipProp"); ok {
			flip = wordOf(v)
		}
		_, hasFlip := b.Field("flipX")
		if prop == sprite || (hasFlip && prop == flip) {
			return errs.New(errs.EditorProps, "track "+trackLabel(target, prop)+" comes from a frames block; expand the frames into keys first")
		}
	}
	return nil
}

func (d *AnimDocument) trackOf(target, prop string) (int, error) {
	if i := d.track(target, prop); i >= 0 {
		return i, nil
	}
	if err := d.framesOwn(target, prop); err != nil {
		return -1, err
	}
	return -1, errs.New(errs.EditorProps, "the clip has no track "+trackLabel(target, prop))
}

func (d *AnimDocument) keySet(op Op) (step, error) {
	t, err := keyTime(op.Key)
	if err != nil {
		return step{}, err
	}
	v, err := op.value()
	if err != nil {
		return step{}, err
	}
	ch := Change{Op: op.Kind, Node: op.Node, Path: op.Path, After: FormatValue(v)}
	i := d.track(op.Node, op.Path)
	if i < 0 {
		if err := d.framesOwn(op.Node, op.Path); err != nil {
			return step{}, err
		}
		if _, ok := anim.ParseKind(op.Comp); !ok {
			return step{}, errs.New(errs.EditorProps, "a new track needs a kind such as float32, vec2 or color", log.F("kind", op.Comp))
		}
		tr := &aetext.Node{Head: "track", Label: op.Node, Attrs: []aetext.Field{
			{Name: "prop", Value: aetext.String(op.Path)},
			{Name: "kind", Value: aetext.Ident(op.Comp)},
		}}
		tr.Blocks = []*aetext.Node{newKey(t, v, op.Values, nil)}
		return d.b.replace(d.b.first("event"), 0, []*aetext.Node{tr}, ch)
	}
	tr := d.b.root.Blocks[i].Clone()
	if k := keyAt(tr, t); k >= 0 {
		if ov, ok := tr.Blocks[k].Attr("v"); ok {
			ch.Before = FormatValue(ov)
		}
		tr.Blocks[k] = newKey(t, v, op.Values, tr.Blocks[k])
	} else {
		insertKey(tr, newKey(t, v, op.Values, nil))
	}
	return d.b.replace(i, 1, []*aetext.Node{tr}, ch)
}

func (d *AnimDocument) keyRemove(op Op) (step, error) {
	t, err := keyTime(op.Key)
	if err != nil {
		return step{}, err
	}
	i, err := d.trackOf(op.Node, op.Path)
	if err != nil {
		return step{}, err
	}
	tr := d.b.root.Blocks[i].Clone()
	k := keyAt(tr, t)
	if k < 0 {
		return step{}, ErrNothing
	}
	ch := Change{Op: op.Kind, Node: op.Node, Path: op.Path}
	if v, ok := tr.Blocks[k].Attr("v"); ok {
		ch.Before = FormatValue(v)
	}
	tr.Blocks = slices.Delete(tr.Blocks, k, k+1)
	if keyAt(tr, t) < 0 && slices.IndexFunc(tr.Blocks, func(b *aetext.Node) bool { return b.Head == "key" }) < 0 {
		return d.b.replace(i, 1, nil, ch)
	}
	return d.b.replace(i, 1, []*aetext.Node{tr}, ch)
}

func (d *AnimDocument) keyMove(op Op) (step, error) {
	from, err := keyTime(op.Key)
	if err != nil {
		return step{}, err
	}
	to, err := keyTime(op.Name)
	if err != nil {
		return step{}, err
	}
	i, err := d.trackOf(op.Node, op.Path)
	if err != nil {
		return step{}, err
	}
	tr := d.b.root.Blocks[i].Clone()
	k := keyAt(tr, from)
	if k < 0 {
		return step{}, ErrNothing
	}
	if sameTime(from, to) {
		return step{}, ErrNothing
	}
	if keyAt(tr, to) >= 0 {
		return step{}, errs.New(errs.EditorProps, "the track already has a key at "+op.Name)
	}
	kb := tr.Blocks[k]
	tr.Blocks = slices.Delete(tr.Blocks, k, k+1)
	setAttr(kb, "t", aetext.Float32(to))
	insertKey(tr, kb)
	return d.b.replace(i, 1, []*aetext.Node{tr}, Change{Op: op.Kind, Node: op.Node, Path: op.Path, Before: op.Key, After: op.Name})
}

func (d *AnimDocument) eventAdd(op Op) (step, error) {
	t, err := keyTime(op.Key)
	if err != nil {
		return step{}, err
	}
	if op.Name == "" {
		return step{}, errs.New(errs.EditorProps, "an event needs a name")
	}
	n := &aetext.Node{Head: "event", Attrs: []aetext.Field{
		{Name: "t", Value: aetext.Float32(t)},
		{Name: "name", Value: aetext.String(op.Name)},
	}}
	if op.hasValue() {
		v, err := op.value()
		if err != nil {
			return step{}, err
		}
		setAttr(n, "payload", v)
	}
	if op.Level != "" {
		setAttr(n, "mode", aetext.Ident(op.Level))
	}
	return d.b.replace(len(d.b.root.Blocks), 0, []*aetext.Node{n}, Change{Op: op.Kind, Node: op.Name, After: op.Key})
}

func (d *AnimDocument) eventEdit(op Op) (step, error) {
	i := d.b.nth("event", op.Index)
	if !op.HasIndex || i < 0 {
		return step{}, errs.New(errs.EditorProps, "the clip has no event "+strconv.Itoa(op.Index+1))
	}
	ev := d.b.root.Blocks[i]
	ch := Change{Op: op.Kind}
	if v, ok := ev.Attr("name"); ok {
		ch.Node = wordOf(v)
	}
	if op.Kind == KindAnimEventRemove {
		return d.b.replace(i, 1, nil, ch)
	}
	t, err := keyTime(op.Key)
	if err != nil {
		return step{}, err
	}
	n := ev.Clone()
	if v, ok := n.Attr("t"); ok {
		ch.Before = FormatValue(v)
	}
	setAttr(n, "t", aetext.Float32(t))
	ch.After = op.Key
	return d.b.replace(i, 1, []*aetext.Node{n}, ch)
}

func (d *AnimDocument) clipSet(op Op) (step, error) {
	switch op.Path {
	case "length", "loop", "fps":
	default:
		return step{}, errs.New(errs.EditorProps, "a clip has length, loop and fps", log.F("attr", op.Path))
	}
	root := d.b.root
	old, had := root.Attr(op.Path)
	inv := Op{Kind: op.Kind, Path: op.Path}
	ch := Change{Op: op.Kind, Path: op.Path}
	if had {
		inv.Value, inv.HasValue = old.Clone(), true
		ch.Before = FormatValue(old)
	}
	redo := Op{Kind: op.Kind, Path: op.Path}
	if !op.hasValue() {
		if op.Path == "length" {
			return step{}, errs.New(errs.EditorProps, "a clip needs a length")
		}
		if !had {
			return step{}, ErrNothing
		}
		dropAttr(root, op.Path)
		return step{inv: inv, redo: redo, change: ch}, nil
	}
	v, err := op.value()
	if err != nil {
		return step{}, err
	}
	setAttr(root, op.Path, v.Clone())
	ch.After = FormatValue(v)
	redo.Value, redo.HasValue = v.Clone(), true
	return step{inv: inv, redo: redo, change: ch}, nil
}

func (d *AnimDocument) framesExpand(op Op) (step, error) {
	root := d.b.root
	i := slices.IndexFunc(root.Blocks, func(b *aetext.Node) bool { return b.Head == "frames" && b.Label == op.Node })
	if i < 0 {
		return step{}, errs.New(errs.EditorProps, "the clip has no frames block for "+trackLabel(op.Node, "frames"))
	}
	tmp := &aetext.Node{Head: anim.Head, Blocks: []*aetext.Node{root.Blocks[i].Clone()}}
	if v, ok := root.Attr("length"); ok {
		tmp.Attrs = []aetext.Field{{Name: "length", Value: v}}
	}
	c, err := anim.Decode(tmp)
	if err != nil {
		return step{}, err
	}
	var tracks []*aetext.Node
	for _, b := range anim.Encode(c).Blocks {
		if b.Head == "track" {
			tracks = append(tracks, b)
		}
	}
	return d.b.replace(i, 1, tracks, Change{Op: op.Kind, Node: op.Node})
}
