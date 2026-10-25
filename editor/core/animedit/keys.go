package animedit

import (
	"cmp"
	"math"
	"slices"
	"strconv"

	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/scene/mutate"
)

type KeyRef struct {
	Track int
	T     float32
}

func timeText(t float32) string {
	return strconv.FormatFloat(float64(t), 'g', -1, 32)
}

func (s *Service) EditClip(label string, ops ...mutate.Op) error {
	if s.clip == nil {
		return errNoClip
	}
	if len(ops) == 0 {
		return nil
	}
	return s.doc.History.Exec(s.edit(label, ops))
}

func (s *Service) track(k KeyRef) (target, prop string, ok bool) {
	if s.clip == nil || k.Track < 0 || k.Track >= len(s.clip.Tracks) {
		return "", "", false
	}
	t := &s.clip.Tracks[k.Track]
	return t.Target, t.Property, true
}

func (s *Service) MoveKeys(keys []KeyRef, delta float32) ([]KeyRef, error) {
	if s.clip == nil || delta == 0 || len(keys) == 0 {
		return keys, nil
	}
	sorted := slices.Clone(keys)
	slices.SortFunc(sorted, func(a, b KeyRef) int {
		if delta > 0 {
			return cmp.Compare(b.T, a.T)
		}
		return cmp.Compare(a.T, b.T)
	})
	ops := make([]mutate.Op, 0, len(sorted))
	out := make([]KeyRef, 0, len(sorted))
	for _, k := range sorted {
		target, prop, ok := s.track(k)
		if !ok {
			continue
		}
		to := mathx.Clamp(k.T+delta, 0, s.clip.Length)
		out = append(out, KeyRef{Track: k.Track, T: to})
		if to != k.T {
			ops = append(ops, mutate.AnimKeyMove(target, prop, timeText(k.T), timeText(to)))
		}
	}
	if err := s.EditClip(i18n.Tr("editor.history.move_keys", "Move Keys"), ops...); err != nil {
		return keys, err
	}
	return out, nil
}

func (s *Service) RemoveKeys(keys []KeyRef) error {
	ops := make([]mutate.Op, 0, len(keys))
	for _, k := range keys {
		if target, prop, ok := s.track(k); ok {
			ops = append(ops, mutate.AnimKeyRemove(target, prop, timeText(k.T)))
		}
	}
	return s.EditClip(i18n.Tr("editor.history.delete_keys", "Delete Keys"), ops...)
}

func (s *Service) SetInterp(keys []KeyRef, interp string) error {
	extra := &aetext.Node{Head: "values", Attrs: []aetext.Field{{Name: "interp", Value: aetext.Ident(interp)}}}
	ops := make([]mutate.Op, 0, len(keys))
	for _, k := range keys {
		target, prop, ok := s.track(k)
		if !ok {
			continue
		}
		v, ok := s.KeyValue(k)
		if !ok {
			continue
		}
		op := mutate.AnimKeySet(target, prop, timeText(k.T), v)
		op.Values = extra
		ops = append(ops, op)
	}
	return s.EditClip(i18n.Tr("editor.history.key_interpolation", "Key Interpolation"), ops...)
}

func wordOf(v aetext.Value) string {
	if v.Kind == aetext.VIdent {
		return v.Ident
	}
	return v.Str
}

func (s *Service) KeyValue(k KeyRef) (aetext.Value, bool) {
	target, prop, ok := s.track(k)
	if !ok || s.adoc == nil {
		return aetext.Value{}, false
	}
	for _, b := range s.adoc.Root().Blocks {
		if b.Head != "track" || b.Label != target {
			continue
		}
		if v, ok := b.Attr("prop"); !ok || wordOf(v) != prop {
			continue
		}
		for _, kb := range b.Blocks {
			if tv, _ := kb.Attr("t"); math.Abs(tv.AsFloat()-float64(k.T)) <= 1e-5 {
				return kb.Attr("v")
			}
		}
	}
	return aetext.Value{}, false
}
