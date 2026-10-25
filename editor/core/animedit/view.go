package animedit

import (
	"cmp"
	"slices"

	"aego/anim"
	"aego/core/curve"
	"aego/core/ids"
	"aego/engine/i18n"
	"aego/scene/mutate"
	"aego/schema"
)

type Row struct {
	Label  string
	Depth  int
	Track  int
	Keys   []float32
	Assets []ids.AssetID
	Step   bool
}

type EventRow struct {
	Index int
	T     float32
	Name  string
	Mode  anim.Mode
}

type View struct {
	Rows   []Row
	Events []EventRow
}

func stepped(t *anim.Track) bool {
	switch t.Kind {
	case schema.KindAssetRef, schema.KindBool, schema.KindEnum:
		return true
	case schema.KindInt32, schema.KindInt64:
		return !t.Numeric
	}
	return len(t.Keys) > 0 && t.Keys[0].Interp == curve.InterpStep
}

func BuildView(c *anim.Clip, d *mutate.AnimDocument) View {
	var v View
	if c == nil {
		return v
	}
	order := make([]int, len(c.Tracks))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(c.Tracks[a].Target, c.Tracks[b].Target) })
	last := "\x00"
	for _, i := range order {
		t := &c.Tracks[i]
		if t.Target != last {
			label := t.Target
			if label == "" {
				label = i18n.Tr("editor.animedit.this_node", "This node")
			}
			v.Rows = append(v.Rows, Row{Label: label, Track: -1})
			last = t.Target
		}
		r := Row{Label: t.Property, Depth: 1, Track: i, Step: stepped(t)}
		for _, k := range t.Keys {
			r.Keys = append(r.Keys, k.T)
			if t.Kind == schema.KindAssetRef {
				r.Assets = append(r.Assets, k.V.A)
			}
		}
		v.Rows = append(v.Rows, r)
	}
	if d == nil {
		return v
	}
	n := 0
	for _, b := range d.Root().Blocks {
		if b.Head != "event" {
			continue
		}
		e := EventRow{Index: n}
		if tv, ok := b.Attr("t"); ok {
			e.T = float32(tv.AsFloat())
		}
		if nv, ok := b.Attr("name"); ok {
			e.Name = wordOf(nv)
		}
		if mv, ok := b.Attr("mode"); ok {
			e.Mode, _ = anim.ParseMode(wordOf(mv))
		}
		v.Events = append(v.Events, e)
		n++
	}
	return v
}
