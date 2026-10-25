package animedit

import (
	"math"
	"slices"

	"aego/anim"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/scene/mutate"
	"aego/scene/propref"
)

// CopiedKey is one key on the animation clipboard, timed from the earliest copied key.
type CopiedKey struct {
	Target string
	Prop   string
	Kind   string
	DT     float32
	Value  aetext.Value
	Attrs  []aetext.Field
}

func (s *Service) keyNode(k KeyRef) *aetext.Node {
	target, prop, ok := s.track(k)
	if !ok || s.adoc == nil {
		return nil
	}
	for _, b := range s.adoc.Root().Blocks {
		if b.Head != "track" || b.Label != target {
			continue
		}
		if v, ok := b.Attr("prop"); !ok || wordOf(v) != prop {
			continue
		}
		for _, kb := range b.Blocks {
			if tv, _ := kb.Attr("t"); kb.Head == "key" && math.Abs(tv.AsFloat()-float64(k.T)) <= 1e-5 {
				return kb
			}
		}
	}
	return nil
}

// CopyKeys puts the keys on the clipboard of the registry, so they paste into any clip.
func (s *Service) CopyKeys(keys []KeyRef) int {
	if s.clip == nil || len(keys) == 0 {
		return 0
	}
	t0 := float32(math.MaxFloat32)
	for _, k := range keys {
		t0 = min(t0, k.T)
	}
	var out []CopiedKey
	for _, k := range keys {
		kb := s.keyNode(k)
		if kb == nil {
			continue
		}
		tr := &s.clip.Tracks[k.Track]
		c := CopiedKey{Target: tr.Target, Prop: tr.Property, Kind: tr.Kind.String(), DT: k.T - t0}
		for _, f := range kb.Attrs {
			switch f.Name {
			case "t":
			case "v":
				c.Value = f.Value.Clone()
			default:
				c.Attrs = append(c.Attrs, aetext.Field{Name: f.Name, Value: f.Value.Clone()})
			}
		}
		out = append(out, c)
	}
	slices.SortStableFunc(out, func(a, b CopiedKey) int {
		switch {
		case a.DT < b.DT:
			return -1
		case a.DT > b.DT:
			return 1
		}
		return 0
	})
	if s.board != nil {
		*s.board = out
	}
	return len(out)
}

// PasteKeys sets the copied keys at the time cursor on the same track paths, creating missing tracks, in one step.
func (s *Service) PasteKeys() error {
	if s.clip == nil {
		return errNoClip
	}
	if s.board == nil || len(*s.board) == 0 {
		return errs.New(errs.EditorCommand, "copy keys first")
	}
	var ops []mutate.Op
	for _, c := range *s.board {
		op := mutate.AnimKeySet(c.Target, c.Prop, timeText(mathx.Clamp(s.time+c.DT, 0, s.clip.Length)), c.Value)
		op.Comp = c.Kind
		if len(c.Attrs) > 0 {
			op.Values = &aetext.Node{Head: "values", Attrs: c.Attrs}
		}
		ops = append(ops, op)
	}
	return s.EditClip(i18n.Tr("editor.history.paste_keys", "Paste Keys"), ops...)
}

// Original is the value of a bound track in the scene, without the clip sampled over it.
func (s *Service) original(b *binding) anim.Value {
	if !b.pose {
		return b.orig
	}
	tr := s.scene.Local(b.node)
	var l [4]float32
	switch b.ref.Part() {
	case propref.PartPosition:
		l = [4]float32{tr.Position.X, tr.Position.Y}
	case propref.PartRotation:
		l = [4]float32{tr.Rotation}
	default:
		l = [4]float32{tr.Scale.X, tr.Scale.Y}
	}
	if lane := b.ref.Lane(); lane >= 0 {
		l = [4]float32{l[lane]}
	}
	return anim.Lanes(l)
}

func same(a, b anim.Value, lanes int) bool {
	if a.I != b.I || a.A != b.A {
		return false
	}
	for i := range lanes {
		if math.Abs(float64(a.L[i]-b.L[i])) > 1e-5 {
			return false
		}
	}
	return true
}

func (s *Service) setOp(b *binding, at string, v anim.Value) mutate.Op {
	return s.setOpKind(&s.clip.Tracks[b.track], at, v)
}

func (s *Service) setOpKind(tr *anim.Track, at string, v anim.Value) mutate.Op {
	op := mutate.AnimKeySet(tr.Target, tr.Property, at, anim.ValueText(tr.Kind, v))
	op.Comp = tr.Kind.String()
	return op
}

// KeyChanged keys every field of the node whose scene value differs from the clip at the current time.
func (s *Service) KeyChanged(node ids.NodeID) (int, error) {
	if !s.applied {
		return 0, errNoClip
	}
	h := s.scene.Handle(node)
	at := s.keyTime()
	local := s.local()
	var ops []mutate.Op
	for i := range s.binds {
		b := &s.binds[i]
		if b.node != h {
			continue
		}
		tr := &s.clip.Tracks[b.track]
		orig := s.original(b)
		if same(orig, s.samp.Track(s.clip, b.track, local), anim.LaneCount(tr.Kind)) {
			continue
		}
		ops = append(ops, s.setOp(b, at, orig))
	}
	if len(ops) == 0 {
		return 0, nil
	}
	return len(ops), s.EditClip(i18n.Tr("editor.history.key_changed", "Key Changed"), ops...)
}

// SetFromScene gives the keys the values their properties have in the scene.
func (s *Service) SetFromScene(keys []KeyRef) error {
	if !s.applied {
		return errNoClip
	}
	var ops []mutate.Op
	for _, k := range keys {
		for i := range s.binds {
			if b := &s.binds[i]; b.track == k.Track {
				ops = append(ops, s.setOp(b, timeText(k.T), s.original(b)))
				break
			}
		}
	}
	return s.EditClip(i18n.Tr("editor.history.set_keys_from_scene", "Set Keys from Scene"), ops...)
}
