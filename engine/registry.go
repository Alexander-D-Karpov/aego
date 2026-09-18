package engine

import (
	"strings"

	"aego/core/errs"
	"aego/core/log"
)

type ModuleInfo struct {
	ID      string
	Version string
}

type Module interface {
	Info() ModuleInfo
	Register(*Registry) error
}

// ModuleIniter is an optional Module interface. Init runs during Build once the
// platform, window and device exist, in Use order, after the registry is frozen.
// Modules that need a live gpu.Device — to compile shaders or allocate buffers —
// implement it so those failures surface from Build rather than in the first frame.
type ModuleIniter interface {
	Init(*Engine) error
}

type Phase uint8

const (
	PhaseFrameBegin Phase = iota
	PhasePollEvents
	PhaseInput
	PhaseFixedBegin
	PhasePrePhysics
	PhasePhysics
	PhasePostPhysics
	PhaseFixedEnd
	PhasePreUpdate
	PhaseUpdate
	PhasePostUpdate
	PhaseAnimation
	PhaseRenderPrepare
	PhaseRender
	PhaseRenderUI
	PhaseFrameEnd
)

const phaseCount = int(PhaseFrameEnd) + 1

var phaseNames = [phaseCount]string{
	"FrameBegin", "PollEvents", "Input", "FixedBegin", "PrePhysics", "Physics",
	"PostPhysics", "FixedEnd", "PreUpdate", "Update", "PostUpdate", "Animation",
	"RenderPrepare", "Render", "RenderUI", "FrameEnd",
}

func (p Phase) String() string {
	if int(p) < phaseCount {
		return phaseNames[p]
	}
	return "UnknownPhase"
}

type Context struct {
	Engine *Engine
	Dt     float64
	Alpha  float64
	Frame  uint64
}

type System interface {
	Run(*Context)
}

type SystemDesc struct {
	ID     string
	Phase  Phase
	Before []string
	After  []string
	System System
}

type SystemRegistry struct {
	reg    *Registry
	descs  []SystemDesc
	index  map[string]int
	phases [phaseCount][]System
	order  [phaseCount][]string
}

func (s *SystemRegistry) Register(d SystemDesc) error {
	if s.reg.frozen {
		return errs.New(errs.EngineFrozen, "registry is frozen", log.F("system", d.ID))
	}
	if d.ID == "" || d.System == nil {
		return errs.New(errs.EngineSystemBad, "system needs an id and an implementation", log.F("system", d.ID))
	}
	if int(d.Phase) >= phaseCount {
		return errs.New(errs.EngineSystemPhase, "unknown phase", log.F("system", d.ID), log.F("phase", int(d.Phase)))
	}
	if _, dup := s.index[d.ID]; dup {
		return errs.New(errs.EngineSystemDup, "duplicate system id", log.F("system", d.ID))
	}
	s.index[d.ID] = len(s.descs)
	s.descs = append(s.descs, d)
	return nil
}

func (s *SystemRegistry) Order(p Phase) []string {
	if int(p) >= phaseCount {
		return nil
	}
	return s.order[p]
}

func (s *SystemRegistry) freeze() error {
	var byPhase [phaseCount][]int
	for i, d := range s.descs {
		byPhase[d.Phase] = append(byPhase[d.Phase], i)
	}
	for p := 0; p < phaseCount; p++ {
		sorted, err := s.sortPhase(Phase(p), byPhase[p])
		if err != nil {
			return err
		}
		for _, di := range sorted {
			s.phases[p] = append(s.phases[p], s.descs[di].System)
			s.order[p] = append(s.order[p], s.descs[di].ID)
		}
	}
	return nil
}

func (s *SystemRegistry) resolve(p Phase, from, id string) (int, error) {
	di, ok := s.index[id]
	if !ok {
		return 0, errs.New(errs.EngineSystemDep, "unknown system in ordering constraint",
			log.F("system", from), log.F("dependency", id))
	}
	if s.descs[di].Phase != p {
		return 0, errs.New(errs.EngineSystemPhase, "ordering constraint crosses phases",
			log.F("system", from), log.F("dependency", id),
			log.F("phase", p.String()), log.F("dependency_phase", s.descs[di].Phase.String()))
	}
	return di, nil
}

func (s *SystemRegistry) sortPhase(p Phase, nodes []int) ([]int, error) {
	n := len(nodes)
	if n == 0 {
		return nil, nil
	}
	local := make(map[int]int, n)
	for li, di := range nodes {
		local[di] = li
	}
	adj := make([][]int, n)
	indeg := make([]int, n)
	edge := func(from, to int) {
		adj[from] = append(adj[from], to)
		indeg[to]++
	}
	for li, di := range nodes {
		d := s.descs[di]
		for _, id := range d.Before {
			other, err := s.resolve(p, d.ID, id)
			if err != nil {
				return nil, err
			}
			edge(li, local[other])
		}
		for _, id := range d.After {
			other, err := s.resolve(p, d.ID, id)
			if err != nil {
				return nil, err
			}
			edge(local[other], li)
		}
	}
	done := make([]bool, n)
	out := make([]int, 0, n)
	for len(out) < n {
		pick := -1
		for li := 0; li < n; li++ {
			if !done[li] && indeg[li] == 0 {
				pick = li
				break
			}
		}
		if pick < 0 {
			var cyc []string
			for li := 0; li < n; li++ {
				if !done[li] {
					cyc = append(cyc, s.descs[nodes[li]].ID)
				}
			}
			return nil, errs.New(errs.EngineSystemCycle, "system ordering has a cycle",
				log.F("phase", p.String()), log.F("systems", strings.Join(cyc, ",")))
		}
		done[pick] = true
		out = append(out, nodes[pick])
		for _, to := range adj[pick] {
			indeg[to]--
		}
	}
	return out, nil
}

type Registry struct {
	frozen  bool
	systems SystemRegistry
	modules []ModuleInfo
}

func newRegistry() *Registry {
	r := &Registry{}
	r.systems.reg = r
	r.systems.index = make(map[string]int)
	return r
}

func (r *Registry) Systems() *SystemRegistry {
	return &r.systems
}

func (r *Registry) Modules() []ModuleInfo {
	return r.modules
}

func (r *Registry) Frozen() bool {
	return r.frozen
}

func (r *Registry) Freeze() error {
	if r.frozen {
		return nil
	}
	if err := r.systems.freeze(); err != nil {
		return err
	}
	r.frozen = true
	return nil
}
