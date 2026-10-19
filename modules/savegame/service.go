package savegame

import (
	"aego/contracts/persistence"
	"aego/contracts/scriptevents"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/engine"
	"aego/format/aetext"
	"aego/save"
	"aego/scene"
	"aego/schema"
)

type reqKind uint8

const (
	reqSave reqKind = iota
	reqLoad
)

type request struct {
	kind reqKind
	slot string
}

type Service struct {
	m         *Module
	world     *engine.World
	scene     *scene.Scene
	marker    schema.TypeID
	hasMarker bool
	base      map[ids.NodeID]bool
	writers   map[scene.NodeHandle]*save.Writer
	readers   map[scene.NodeHandle]*save.Reader
	reqs      []request
	since     float64
	applied   bool
	capturing bool
	vals      []persistence.ScriptValue
}

var (
	_ persistence.Service = (*Service)(nil)
	_ persistence.Flusher = (*Service)(nil)
)

func NewService(m *Module, w *engine.World, s *scene.Scene) *Service {
	svc := &Service{
		m:       m,
		world:   w,
		scene:   s,
		base:    map[ids.NodeID]bool{},
		writers: map[scene.NodeHandle]*save.Writer{},
		readers: map[scene.NodeHandle]*save.Reader{},
	}
	if td, ok := s.Registry().ByName(MarkerName); ok {
		svc.marker, svc.hasMarker = td.ID, true
	}
	root := s.Root()
	for _, h := range s.PreOrder() {
		if h != root {
			svc.base[s.ID(h)] = true
		}
	}
	return svc
}

func (s *Service) spawner() persistence.Spawner {
	if s.world == nil {
		return nil
	}
	sp, _ := engine.ServiceOf[persistence.Spawner](s.world, persistence.SpawnerID)
	return sp
}

func (s *Service) scripts() persistence.Scripts {
	if s.world == nil {
		return nil
	}
	ps, _ := engine.ServiceOf[persistence.Scripts](s.world, persistence.ScriptsID)
	return ps
}

func (s *Service) sink() scriptevents.Sink {
	if s.world == nil {
		return nil
	}
	sk, _ := engine.ServiceOf[scriptevents.Sink](s.world, scriptevents.ServiceID)
	return sk
}

func (s *Service) host() persistence.Host {
	if h, ok := engine.ServiceOf[persistence.Host](s.world, persistence.HostID); ok {
		return h
	}
	if s.m.eng != nil {
		h, _ := engine.Service[persistence.Host](s.m.eng, persistence.HostID)
		return h
	}
	return nil
}

func (s *Service) marked(h scene.NodeHandle) (*Persist, bool) {
	if !s.hasMarker {
		return nil, false
	}
	p, ok := s.scene.GetByType(h, s.marker)
	if !ok {
		return nil, false
	}
	return (*Persist)(p), true
}

func badSlot(slot string) error {
	return errs.New(errs.PlatformConfig, "slot name may use letters, digits, - and _", log.F("slot", slot))
}

func (s *Service) Save(slot string) error {
	if !save.ValidSlot(slot) {
		return badSlot(slot)
	}
	s.reqs = append(s.reqs, request{kind: reqSave, slot: slot})
	return nil
}

func (s *Service) Load(slot string) error {
	if !save.ValidSlot(slot) {
		return badSlot(slot)
	}
	st := s.m.store()
	if st == nil {
		return errs.New(errs.VFSIO, "saves are not available in this run")
	}
	if !st.Exists(slot) {
		return errs.New(errs.VFSIO, "there is no save in slot "+slot, log.F("slot", slot))
	}
	s.reqs = append(s.reqs, request{kind: reqLoad, slot: slot})
	return nil
}

func (s *Service) Checkpoint() error {
	return s.Save(SlotCheckpoint)
}

func (s *Service) SetAutosave(seconds float64) {
	s.m.autosave = max(seconds, 0)
	s.since = 0
}

func (s *Service) Exists(slot string) bool {
	st := s.m.store()
	return st != nil && st.Exists(slot)
}

func (s *Service) Delete(slot string) error {
	st := s.m.store()
	if st == nil {
		return errs.New(errs.VFSIO, "saves are not available in this run")
	}
	return st.Delete(slot)
}

func (s *Service) Step(dt, real float64) {
	s.m.playtime += dt
	if s.applied {
		clear(s.readers)
		s.applied = false
	}
	if a := s.m.autosave; a > 0 {
		s.since += real
		if s.since >= a {
			s.since = 0
			s.reqs = append(s.reqs, request{kind: reqSave, slot: SlotAutosave})
		}
	}
	s.Flush()
}

func (s *Service) Flush() {
	if len(s.reqs) == 0 {
		return
	}
	reqs := s.reqs
	s.reqs = nil
	for _, r := range reqs {
		if r.kind == reqLoad {
			s.read(r.slot)
			return
		}
		s.write(r.slot)
	}
}

func (s *Service) write(slot string) {
	if s.world.Replaying() {
		s.Capture(slot)
		return
	}
	st := s.m.store()
	if st == nil {
		s.m.finish(Result{Slot: slot, Err: errs.New(errs.VFSIO, "saves are not available in this run")})
		return
	}
	doc, err := s.Capture(slot)
	if err != nil {
		s.m.finish(Result{Slot: slot, Err: err})
		return
	}
	s.m.encode(st, slot, doc)
}

func (s *Service) read(slot string) {
	if s.world.Replaying() {
		return
	}
	st, host := s.m.store(), s.host()
	switch {
	case st == nil:
		s.m.finish(Result{Slot: slot, Load: true, Err: errs.New(errs.VFSIO, "saves are not available in this run")})
		return
	case host == nil:
		s.m.finish(Result{Slot: slot, Load: true, Err: errs.New(errs.EngineModule, "this run cannot switch scenes, so it cannot load games")})
		return
	}
	doc, err := st.Load(slot)
	if err != nil {
		s.m.finish(Result{Slot: slot, Load: true, Err: err})
		return
	}
	doc.Meta.Name = slot
	m := s.m
	m.pending = doc
	call := func() {
		if err := host.LoadScene(doc.Meta.Scene); err != nil {
			m.pending = nil
			m.finish(Result{Slot: slot, Load: true, Err: err})
		}
	}
	if m.eng != nil {
		m.eng.Jobs().OnMain(call)
		return
	}
	call()
}

func (s *Service) writerFor(h scene.NodeHandle) *save.Writer {
	w := s.writers[h]
	if w == nil {
		w = save.NewWriter(&aetext.Node{Head: "data"}, s)
		s.writers[h] = w
	}
	return w
}

func (s *Service) readerFor(h scene.NodeHandle) *save.Reader {
	return s.readers[h]
}
