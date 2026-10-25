package addpalette

import (
	"sort"

	"aego/editor/core/context"
	"aego/editor/core/state"
	"aego/format/aetext"
)

const storeVersion = 1

var modeNames = [modeCount]string{"components", "create"}

type modeState struct {
	recent []string
	favs   []string
	open   []string
}

type store struct {
	modes [modeCount]modeState
	uses  map[string]int
}

func (s *Service) load() {
	if s.loaded || s.d.Root == "" {
		return
	}
	s.loaded = true
	data, ok := state.LoadPalette(s.d.Root)
	if !ok {
		return
	}
	doc, err := aetext.Parse(data, state.PaletteFile)
	if err != nil || doc.Head != "palette" {
		return
	}
	strs := func(n *aetext.Node, name string) []string {
		v, _ := n.Attr(name)
		var out []string
		for _, x := range v.List {
			out = append(out, x.Str)
		}
		return out
	}
	for m, name := range modeNames {
		doc.EachBlock(name, func(n *aetext.Node) {
			s.store.modes[m] = modeState{recent: strs(n, "recent"), favs: strs(n, "favorites"), open: strs(n, "open")}
		})
	}
	s.store.uses = map[string]int{}
	doc.EachBlock("use", func(n *aetext.Node) {
		k, _ := n.Attr("key")
		c, _ := n.Attr("count")
		if k.Str != "" && c.Int > 0 {
			s.store.uses[k.Str] = int(c.Int)
		}
	})
}

func (s *Service) save() {
	if s.d.Root == "" {
		return
	}
	list := func(xs []string) aetext.Value {
		vs := make([]aetext.Value, len(xs))
		for i, x := range xs {
			vs[i] = aetext.String(x)
		}
		return aetext.List(vs...)
	}
	root := &aetext.Node{Head: "palette"}
	root.Attrs = append(root.Attrs, aetext.Field{Name: "version", Value: aetext.Int(storeVersion)})
	for m, name := range modeNames {
		st := &s.store.modes[m]
		n := &aetext.Node{Head: name}
		n.Attrs = append(n.Attrs,
			aetext.Field{Name: "recent", Value: list(st.recent)},
			aetext.Field{Name: "favorites", Value: list(st.favs)},
			aetext.Field{Name: "open", Value: list(st.open)})
		root.Blocks = append(root.Blocks, n)
	}
	keys := make([]string, 0, len(s.store.uses))
	for k := range s.store.uses {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		root.Blocks = append(root.Blocks, &aetext.Node{Head: "use", Attrs: []aetext.Field{
			{Name: "key", Value: aetext.String(k)}, {Name: "count", Value: aetext.Int(int64(s.store.uses[k]))}}})
	}
	if err := state.SavePalette(s.d.Root, aetext.Write(root)); err != nil {
		s.d.Ctx.Notify(context.LevelWarn, "cannot save the palette state: "+err.Error())
	}
}
