package savegame

import (
	"aego/contracts/scriptevents"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/save"
	"aego/scene"
)

type placed struct {
	h scene.NodeHandle
	n *aetext.Node
}

func (s *Service) warn(msg string, fields ...log.Field) {
	s.m.logger().Warn(msg, fields...)
}

func (s *Service) Apply(doc *save.Doc) error {
	sc := s.scene
	for _, id := range doc.Destroyed {
		if h := sc.Handle(id); !h.IsZero() && sc.IsAlive(h) {
			sc.Destroy(h)
		}
	}
	var spawned []placed
	sp := s.spawner()
	switch {
	case sp != nil:
		sp.SetCounter(doc.Meta.Counter)
		for _, b := range doc.Spawned {
			pid, err := ids.ParseNodeID(b.Label)
			if err != nil {
				s.warn("a saved object has a malformed id", log.F("id", b.Label))
				continue
			}
			r := save.NewReader(b, s)
			parent, _ := r.Ref("parent")
			h, err := sp.SpawnAs(r.Asset("prefab"), parent, mathx.Vec2{}, 0, pid)
			if err != nil {
				s.warn("a saved object cannot be spawned", log.F("id", b.Label), log.F("err", err.Error()))
				continue
			}
			spawned = append(spawned, placed{h: h, n: b})
		}
	case len(doc.Spawned) > 0:
		s.warn("saved objects are skipped, the spawn module is not running")
	}
	if ps := s.scripts(); ps != nil {
		ps.Prepare()
	}
	for _, b := range doc.Nodes {
		id, err := ids.ParseNodeID(b.Label)
		h := sc.Handle(id)
		if err != nil || h.IsZero() || !sc.IsAlive(h) {
			s.warn("a saved node is not in the scene", log.F("node", b.Label))
			continue
		}
		s.applyContent(h, b)
	}
	for _, p := range spawned {
		s.applyContent(p.h, p.n)
		for _, c := range p.n.Blocks {
			if c.Head != "node" {
				continue
			}
			h := sc.Find(p.h, c.Label)
			if h.IsZero() || !sc.IsAlive(h) {
				s.warn("a saved child is not in its prefab", log.F("path", c.Label))
				continue
			}
			s.applyContent(h, c)
		}
	}
	s.m.global.Fields = cloneFields(doc.Global)
	s.loadSections(doc)
	s.m.playtime = doc.Meta.Playtime
	s.emitLoad()
	s.applied = true
	return nil
}

func (s *Service) applyContent(h scene.NodeHandle, n *aetext.Node) {
	sc := s.scene
	r := save.NewReader(n, s)
	if v, ok := n.Field("enabled"); ok && v.Kind == aetext.VBool {
		sc.SetEnabled(h, v.Bool)
	}
	applyTransform(sc, h, r)
	for _, b := range n.Blocks {
		switch b.Head {
		case "component":
			s.applyComponent(h, b)
		case "script":
			s.applyScript(h, b)
		case "data":
			s.readers[h] = save.NewReader(b, s)
		}
	}
}

func (s *Service) applyComponent(h scene.NodeHandle, b *aetext.Node) {
	sc := s.scene
	reg := sc.Registry()
	td, ok := reg.ByName(b.Label)
	if !ok {
		s.warn("a saved component is not registered", log.F("type", b.Label))
		return
	}
	p, ok := sc.GetByType(h, td.ID)
	if !ok {
		s.warn("a saved component is not on its node", log.F("type", b.Label), log.F("node", sc.Path(h)))
		return
	}
	for _, f := range b.Fields {
		fd, ok := td.Field(f.Name)
		if !ok {
			continue
		}
		v, ok := s.readField(reg, fd, f.Value)
		if !ok {
			continue
		}
		if err := fd.Set(p, v); err != nil {
			s.warn("a saved field cannot be restored", log.F("field", td.FullName+"."+f.Name), log.F("err", err.Error()))
		}
	}
}

func (s *Service) applyScript(h scene.NodeHandle, b *aetext.Node) {
	ps := s.scripts()
	if ps == nil {
		return
	}
	s.vals = ps.Persisted(h, s.vals[:0])
	for _, f := range b.Fields {
		found := false
		for _, cur := range s.vals {
			if cur.Script != b.Label || cur.Field != f.Name {
				continue
			}
			v, text, ok := s.fromScript(cur.Kind, f.Value)
			if ok {
				cur.Value, cur.Text = v, text
				found = ps.RestoreField(h, cur)
			}
			break
		}
		if !found {
			s.warn("a saved script field is not restored", log.F("field", b.Label+"."+f.Name))
		}
	}
}

func (s *Service) loadSections(doc *save.Doc) {
	known := map[string]bool{}
	for _, o := range s.m.sections {
		known[o.ID] = true
		sec, ok := doc.Section(o.ID)
		if !ok {
			continue
		}
		if v := o.Section.Version(); sec.Version != v {
			if err := o.Section.Migrate(sec.Node, sec.Version, v); err != nil {
				s.warn("a save section cannot be migrated", log.F("section", o.ID), log.F("err", err.Error()))
				continue
			}
		}
		if err := o.Section.Load(s.world, save.NewReader(sec.Node, s)); err != nil {
			s.warn("a save section cannot be loaded", log.F("section", o.ID), log.F("err", err.Error()))
		}
	}
	s.m.unknown = s.m.unknown[:0]
	for _, sec := range doc.Sections {
		if !known[sec.ID] {
			s.m.unknown = append(s.m.unknown, sec)
		}
	}
}

func (s *Service) emitLoad() {
	sink := s.sink()
	if sink == nil || s.m.ev.Load == scriptevents.NoEvent {
		return
	}
	l, ok := sink.(scriptevents.Listener)
	if !ok {
		return
	}
	sc := s.scene
	root := sc.Root()
	for _, h := range sc.PreOrder() {
		if h != root && sc.IsAlive(h) && l.ListensOn(h, s.m.ev.Load) {
			sink.Emit(h, s.m.ev.Load, nil)
		}
	}
}
