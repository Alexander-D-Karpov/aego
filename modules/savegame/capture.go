package savegame

import (
	"errors"
	"slices"
	"strings"
	"time"

	"aego/contracts/scriptevents"
	"aego/core/ids"
	"aego/engine"
	"aego/format/aetext"
	"aego/save"
	"aego/scene"
	"aego/schema"
)

func hasContent(n *aetext.Node) bool {
	return len(n.Attrs) > 0 || len(n.Fields) > 0 || len(n.Blocks) > 0
}

func (s *Service) collectSaves() {
	clear(s.writers)
	sink := s.sink()
	if sink == nil || s.m.ev.Save == scriptevents.NoEvent {
		return
	}
	l, ok := sink.(scriptevents.Listener)
	if !ok {
		return
	}
	sc := s.scene
	root := sc.Root()
	sent := false
	for _, h := range sc.PreOrder() {
		if h != root && sc.IsAlive(h) && l.ListensOn(h, s.m.ev.Save) {
			sink.Emit(h, s.m.ev.Save, nil)
			sent = true
		}
	}
	if sent {
		s.capturing = true
		sink.Flush()
		s.capturing = false
	}
}

func (s *Service) content(h scene.NodeHandle, n *aetext.Node, spawned bool) {
	sc := s.scene
	mark, marked := s.marked(h)
	if spawned || (marked && !mark.NoEnabled) {
		n.Fields = append(n.Fields, aetext.Field{Name: "enabled", Value: aetext.Bool(sc.Enabled(h))})
	}
	if spawned || (marked && !mark.NoTransform) {
		n.Blocks = append(n.Blocks, transformNode(sc, h))
	}
	s.components(h, n)
	s.scriptFields(h, n)
	if w := s.writers[h]; w != nil && !w.Empty() {
		n.Blocks = append(n.Blocks, w.Node())
	}
}

func (s *Service) components(h scene.NodeHandle, n *aetext.Node) {
	sc := s.scene
	reg := sc.Registry()
	sc.ComponentsOf(h, func(t schema.TypeID) {
		td, ok := reg.ByID(t)
		if !ok {
			return
		}
		p, ok := sc.GetByType(h, t)
		if !ok {
			return
		}
		var c *aetext.Node
		for i := range td.Fields {
			fd := &td.Fields[i]
			if !fd.Flags.Has(schema.FlagPersist) {
				continue
			}
			var v schema.Value
			fd.Get(p, &v)
			av, ok := s.fieldValue(reg, fd, v)
			if !ok {
				continue
			}
			if c == nil {
				c = &aetext.Node{Head: "component", Label: td.FullName}
				n.Blocks = append(n.Blocks, c)
			}
			c.Fields = append(c.Fields, aetext.Field{Name: fd.Name, Value: av})
		}
	})
}

func (s *Service) scriptFields(h scene.NodeHandle, n *aetext.Node) {
	ps := s.scripts()
	if ps == nil {
		return
	}
	s.vals = ps.Persisted(h, s.vals[:0])
	var cur *aetext.Node
	for _, sv := range s.vals {
		av, ok := s.scriptValue(sv)
		if !ok {
			continue
		}
		if cur == nil || cur.Label != sv.Script {
			cur = &aetext.Node{Head: "script", Label: sv.Script}
			n.Blocks = append(n.Blocks, cur)
		}
		cur.Fields = append(cur.Fields, aetext.Field{Name: sv.Field, Value: av})
	}
}

func (s *Service) children(root, h scene.NodeHandle, n *aetext.Node) {
	sc := s.scene
	for c := sc.FirstChild(h); !c.IsZero(); c = sc.NextSibling(c) {
		if !sc.IsAlive(c) {
			continue
		}
		child := &aetext.Node{Head: "node", Label: s.relPath(root, c)}
		s.content(c, child, false)
		if hasContent(child) {
			n.Blocks = append(n.Blocks, child)
		}
		s.children(root, c, n)
	}
}

func (s *Service) Capture(slot string) (*save.Doc, error) {
	sc := s.scene
	s.collectSaves()
	defer clear(s.writers)
	doc := &save.Doc{}
	root := sc.Root()
	for _, h := range sc.PreOrder() {
		if h == root || !sc.IsAlive(h) {
			continue
		}
		id := sc.ID(h)
		if !s.base[id] {
			continue
		}
		if _, _, under := s.spawnRoot(h); under {
			continue
		}
		n := &aetext.Node{Head: "node", Label: id.String()}
		s.content(h, n, false)
		if hasContent(n) {
			doc.Nodes = append(doc.Nodes, n)
		}
	}
	for id := range s.base {
		if h := sc.Handle(id); h.IsZero() || !sc.IsAlive(h) {
			doc.Destroyed = append(doc.Destroyed, id)
		}
	}
	slices.SortFunc(doc.Destroyed, func(a, b ids.NodeID) int { return strings.Compare(a.String(), b.String()) })
	if sp := s.spawner(); sp != nil {
		sp.EachSpawned(func(h scene.NodeHandle, prefab ids.AssetID, pid ids.NodeID) {
			n := &aetext.Node{Head: "spawned", Label: pid.String()}
			n.Fields = append(n.Fields, aetext.Field{Name: "prefab", Value: aetext.AssetRef(prefab.String())})
			if p := sc.Parent(h); p != root {
				if v, ok := s.RefValue(p); ok {
					n.Fields = append(n.Fields, aetext.Field{Name: "parent", Value: v})
				}
			}
			s.content(h, n, true)
			s.children(h, h, n)
			doc.Spawned = append(doc.Spawned, n)
		})
		doc.Meta.Counter = sp.Counter()
	}
	doc.Global = cloneFields(s.m.global.Fields)
	var errsList []error
	known := map[string]bool{}
	for _, o := range s.m.sections {
		known[o.ID] = true
		w := save.NewWriter(&aetext.Node{}, s)
		if err := o.Section.Save(s.world, w); err != nil {
			errsList = append(errsList, err)
			continue
		}
		doc.Sections = append(doc.Sections, save.Section{ID: o.ID, Version: o.Section.Version(), Node: w.Node()})
	}
	for _, u := range s.m.unknown {
		if !known[u.ID] {
			doc.Sections = append(doc.Sections, u)
		}
	}
	doc.Meta.Name = slot
	doc.Meta.Game = s.m.game
	doc.Meta.Engine = engine.Version
	if h := s.host(); h != nil {
		doc.Meta.Scene = h.SceneID()
	}
	doc.Meta.Date = time.Now()
	doc.Meta.Playtime = s.m.playtime
	doc.Meta.Objects = len(doc.Nodes) + len(doc.Spawned)
	return doc, errors.Join(errsList...)
}
