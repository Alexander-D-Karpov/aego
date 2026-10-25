package animedit

import (
	"aego/anim"
	"aego/core/curve"
	"aego/format/aetext"
	"aego/scene/mutate"
)

// Lanes is the number of curve lanes of a track, 0 when it has none to draw.
func (s *Service) Lanes(track int) int {
	if s.clip == nil || track < 0 || track >= len(s.clip.Tracks) {
		return 0
	}
	t := &s.clip.Tracks[track]
	if !t.Numeric && anim.LaneCount(t.Kind) == 0 {
		return 0
	}
	return max(anim.LaneCount(t.Kind), 1)
}

// LaneCurve is one lane of a numeric track as a curve.
func (s *Service) LaneCurve(track, lane int) curve.Curve {
	var cv curve.Curve
	if s.Lanes(track) == 0 {
		return cv
	}
	for _, k := range s.clip.Tracks[track].Keys {
		v := k.V.L[lane]
		if anim.LaneCount(s.clip.Tracks[track].Kind) == 0 {
			v = float32(k.V.I)
		}
		cv.Keys = append(cv.Keys, curve.Key{T: k.T, V: v, In: k.In[lane], Out: k.Out[lane], Interp: k.Interp, Tangent: k.Tangent})
	}
	return cv
}

// CurveOps turns an edit of one lane into key ops; keys are matched by index and the other lanes keep their values.
func (s *Service) CurveOps(track, lane int, before, after curve.Curve) []mutate.Op {
	if s.Lanes(track) == 0 {
		return nil
	}
	tr := &s.clip.Tracks[track]
	var ops []mutate.Op
	switch {
	case len(after.Keys) < len(before.Keys):
		i := firstDiff(before, after)
		ops = append(ops, mutate.AnimKeyRemove(tr.Target, tr.Property, timeText(before.Keys[i].T)))
	case len(after.Keys) > len(before.Keys):
		i := firstDiff(after, before)
		var smp anim.Sampler
		smp.Reset(s.clip)
		base := anim.Key{V: smp.Track(s.clip, track, after.Keys[i].T), Interp: after.Keys[i].Interp}
		ops = append(ops, s.laneSet(tr, base, lane, after.Keys[i], true))
	default:
		for i := range after.Keys {
			b, a := before.Keys[i], after.Keys[i]
			if b == a || i >= len(tr.Keys) {
				continue
			}
			if a.T != b.T {
				ops = append(ops, mutate.AnimKeyMove(tr.Target, tr.Property, timeText(b.T), timeText(a.T)))
			}
			if a.V != b.V || a.Tangent != b.Tangent || (a.Tangent == curve.TangentFree && (a.In != b.In || a.Out != b.Out)) {
				ops = append(ops, s.laneSet(tr, tr.Keys[i], lane, a, a.Tangent != b.Tangent))
			}
		}
	}
	return ops
}

func firstDiff(long, short curve.Curve) int {
	for i := range short.Keys {
		if long.Keys[i].T != short.Keys[i].T {
			return i
		}
	}
	return len(short.Keys)
}

func (s *Service) laneSet(tr *anim.Track, base anim.Key, lane int, k curve.Key, tangent bool) mutate.Op {
	v := base.V
	if anim.LaneCount(tr.Kind) == 0 {
		v.I = int64(k.V)
	} else {
		v.L[lane] = k.V
	}
	op := mutate.AnimKeySet(tr.Target, tr.Property, timeText(k.T), anim.ValueText(tr.Kind, v))
	op.Comp = tr.Kind.String()
	extra := &aetext.Node{Head: "values"}
	if k.Tangent == curve.TangentFree && anim.LaneCount(tr.Kind) > 0 {
		in, out := base.In, base.Out
		in[lane], out[lane] = k.In, k.Out
		n := anim.LaneCount(tr.Kind)
		extra.Attrs = append(extra.Attrs,
			aetext.Field{Name: "tangent", Value: aetext.Ident(k.Tangent.String())},
			aetext.Field{Name: "in", Value: lanesText(in, n)},
			aetext.Field{Name: "out", Value: lanesText(out, n)})
	} else if tangent {
		extra.Attrs = append(extra.Attrs, aetext.Field{Name: "tangent", Value: aetext.Ident(k.Tangent.String())})
	}
	if len(extra.Attrs) > 0 {
		op.Values = extra
	}
	return op
}

func lanesText(l [4]float32, n int) aetext.Value {
	if n == 1 {
		return aetext.Float32(l[0])
	}
	items := make([]aetext.Value, n)
	for i := range items {
		items[i] = aetext.Float32(l[i])
	}
	return aetext.List(items...)
}
