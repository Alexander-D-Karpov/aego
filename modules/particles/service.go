package particles

import (
	"aego/engine/problems"
	"errors"
	"time"

	"aego/core/mathx"
	"aego/core/rewind"
	"aego/engine"
	"aego/render/sprite"
	"aego/scene"
	"aego/schema"
)

const MaxWorldCapacity = 4 * MaxCapacity

var errWorldCapacity = errors.New("particle systems exceed the world pool limit of 262144 slots")

type Stats struct {
	Emitters, Alive, Capacity, Submitted, VisibleEmitters, CulledEmitters                               int
	BornThisFrame, DroppedThisFrame, SimulateNS, ExtractNS, SortNS, InstanceUploads, UploadBytes, Draws uint64
	Requested, Spawned, Dropped                                                                         uint64
}

type emitter struct {
	pool                Pool
	cfg, attempt        ParticleSystem2D
	mask                []mathx.Vec2
	order               []sortParticle
	seen                uint64
	generation          uint32
	tried, dirty, valid bool
	err                 error
}

type completion struct {
	node    scene.NodeHandle
	emitter *emitter
}

type Service struct {
	completions                          []completion
	allocated                            int
	issues                               uint64
	unsubscribeProblems                  func()
	reduced                              bool
	metrics                              Stats
	usage                                *sprite.SubmissionStats
	pendingBirths, pendingDrops          uint64
	world                                *engine.World
	scene                                *scene.Scene
	module                               *Module
	typ                                  schema.TypeID
	enabled                              bool
	systems                              map[scene.NodeHandle]*emitter
	activations                          map[scene.NodeHandle]uint64
	serial, sessionSeed                  uint64
	submitted                            int
	unsubscribeScene, unsubscribeSetting func()
}

func Of(w *engine.World) *Service {
	s, _ := engine.ServiceOf[*Service](w, ServiceID)
	return s
}

func newService(w *engine.World, sc *scene.Scene, m *Module) *Service {
	s := &Service{world: w, scene: sc, module: m, usage: new(sprite.SubmissionStats), typ: scene.Store[ParticleSystem2D](sc).Type(), enabled: true,
		systems: make(map[scene.NodeHandle]*emitter), activations: make(map[scene.NodeHandle]uint64)}
	if w.Random != nil {
		s.sessionSeed = w.Random.Seed()
	}
	s.unsubscribeScene = sc.Events().Subscribe(s.onEvent)
	if m.problems != nil {
		s.unsubscribeProblems = m.problems.Add(problems.Source{Revision: func() uint64 { return s.issues }, Append: s.diagnostics})
	}
	return s
}

func (s *Service) release(h scene.NodeHandle) {
	if e := s.systems[h]; e != nil {
		s.allocated -= cap(e.pool.Particles())
		if e.err != nil {
			s.issues++
		}
		delete(s.systems, h)
	}
}
func (s *Service) diagnostics(out []problems.Diagnostic) []problems.Diagnostic {
	for h, e := range s.systems {
		if e.err != nil {
			out = append(out, problems.Diagnostic{Owner: ID, Code: "PRT001", Severity: problems.Warning, Message: e.err.Error(), Help: "Correct the emitter configuration or reduce its reservation. A previous working configuration remains active.", Subject: problems.Subject{Kind: problems.SubjectNode, Scene: s.scene, Node: s.scene.ID(h)}})
		}
	}
	return out
}
func (s *Service) onEvent(ev scene.Event) {
	switch ev.Kind {
	case scene.EvNodeDestroyed:
		s.release(ev.Node)
		delete(s.activations, ev.Node)
	case scene.EvComponentRemoved:
		if ev.Type == s.typ {
			s.release(ev.Node)
		}
	case scene.EvPropertyChanged:
		if e := s.systems[ev.Node]; ev.Type == s.typ && e != nil {
			e.dirty = true
		}
	}
}

func (s *Service) close() {
	if s.unsubscribeProblems != nil {
		s.unsubscribeProblems()
		s.unsubscribeProblems = nil
	}
	if s.unsubscribeSetting != nil {
		s.unsubscribeSetting()
		s.unsubscribeSetting = nil
	}
	if s.unsubscribeScene != nil {
		s.unsubscribeScene()
		s.unsubscribeScene = nil
	}
	s.SetEnabled(false)
}

func (s *Service) Enabled() bool { return s.enabled }

func (s *Service) SetEnabled(on bool) {
	if s.enabled == on {
		return
	}
	s.enabled = on
	if !on {
		s.reset()
	}
}

func (s *Service) reset() {
	clear(s.systems)
	clear(s.activations)
	s.allocated = 0
	s.issues++
	s.metrics = Stats{}
	*s.usage = sprite.SubmissionStats{}
	s.pendingBirths, s.pendingDrops = 0, 0
	s.submitted = 0
}

type transientKey struct{}

func (transientKey) Size() int { return 0 }

// There is no particle state in a keyframe. Restore only discards the old visual timeline.
func (*Service) Capture(rewind.Key) rewind.Key { return transientKey{} }
func (s *Service) Restore(rewind.Key)          { s.reset() }
func (*Service) Hash(*rewind.Hasher)           {}

func (s *Service) seed(h scene.NodeHandle, c *ParticleSystem2D) uint64 {
	s.activations[h]++
	if c.Seed != 0 {
		return uint64(c.Seed)
	}
	if s.world.Random != nil {
		s.sessionSeed = s.world.Random.Seed()
	}
	return StableSeed(s.sessionSeed, s.scene.ID(h), s.activations[h])
}

func (s *Service) state(h scene.NodeHandle, c *ParticleSystem2D) *emitter {
	e := s.systems[h]
	fresh := e == nil
	if fresh {
		e = &emitter{dirty: true}
		s.systems[h] = e
	}
	oldErr := e.err
	oldCap := cap(e.pool.Particles())
	if int64(c.MaxParticles) > int64(MaxWorldCapacity-s.allocated+oldCap) {
		e.err = errWorldCapacity
		e.dirty = true
	} else {
		if reset, _ := e.configure(c, s.scene.Resolver()); reset {
			e.start(s.seed(h, &e.cfg), s.reduced, s.scene.RenderWorld(h), e.cfg.PlayOnStart)
			s.pendingBirths += e.pool.Spawned
			s.pendingDrops += e.pool.Dropped
		}
		s.allocated += cap(e.pool.Particles()) - oldCap
	}
	if (oldErr == nil) != (e.err == nil) || oldErr != nil && e.err != nil && oldErr.Error() != e.err.Error() {
		s.issues++
	}

	if e.valid {
		e.pool.SetReduced(&e.cfg, s.reduced)
	}

	e.seen = s.serial
	return e
}

func (s *Service) target(h scene.NodeHandle) (*emitter, *ParticleSystem2D) {
	if !s.enabled || s.world.Replaying() || !s.scene.IsAlive(h) || !s.scene.EffectiveEnabled(h) {
		return nil, nil
	}
	c, ok := scene.Get[ParticleSystem2D](s.scene, h)
	if !ok {
		return nil, nil
	}
	e := s.state(h, c)
	if !e.valid {
		return nil, nil
	}
	return e, &e.cfg
}

func (s *Service) Play(h scene.NodeHandle) {
	if e, c := s.target(h); e != nil && !e.pool.Emitting() {
		e.pool.Reseed(s.seed(h, c))
		born, drop := e.pool.Spawned, e.pool.Dropped
		e.play(s.scene.RenderWorld(h))
		s.pendingBirths += e.pool.Spawned - born
		s.pendingDrops += e.pool.Dropped - drop
	}
}
func (s *Service) Restart(h scene.NodeHandle) {
	if e, c := s.target(h); e != nil {
		e.start(s.seed(h, c), s.reduced, s.scene.RenderWorld(h), true)
		s.pendingBirths += e.pool.Spawned
		s.pendingDrops += e.pool.Dropped
	}
}
func (s *Service) Burst(h scene.NodeHandle, n int) {
	if n <= 0 {
		return
	}
	if e, c := s.target(h); e != nil {
		born, drop := e.pool.Spawned, e.pool.Dropped
		e.pool.Burst(c, n, s.scene.World(h))
		s.pendingBirths += e.pool.Spawned - born
		s.pendingDrops += e.pool.Dropped - drop
	}
}
func (s *Service) Stop(h scene.NodeHandle, clear bool) {
	if e, _ := s.target(h); e != nil {
		e.pool.Stop(clear)
	}
}
func (s *Service) Clear(h scene.NodeHandle) {
	if e, _ := s.target(h); e != nil {
		e.pool.Clear()
	}
}
func (s *Service) Count(h scene.NodeHandle) int {
	if e := s.systems[h]; e != nil {
		return len(e.pool.Particles())
	}
	return 0
}
func (s *Service) Emitted(h scene.NodeHandle) uint64 {
	if e := s.systems[h]; e != nil {
		return e.pool.Spawned
	}
	return 0
}
func (s *Service) Playing(h scene.NodeHandle) bool {
	e := s.systems[h]
	return e != nil && e.pool.Playing()
}
func (s *Service) Emitting(h scene.NodeHandle) bool {
	e := s.systems[h]
	return e != nil && e.pool.Emitting()
}

func (s *Service) Stats() Stats {
	stats := s.metrics
	stats.Submitted = s.submitted
	stats.InstanceUploads, stats.UploadBytes, stats.Draws = s.usage.Uploads, s.usage.Bytes, s.usage.Draws
	for _, e := range s.systems {
		if !e.valid {
			continue
		}
		stats.Emitters++
		stats.Alive += len(e.pool.Particles())
		stats.Capacity += cap(e.pool.Particles())
		stats.Spawned += e.pool.Spawned
		stats.Requested += e.pool.Requested
		stats.Dropped += e.pool.Dropped
	}
	return stats
}

type simulate struct{}

func (simulate) Run(ctx *engine.Context) {
	if s := Of(ctx.World); s != nil {
		if ctx.Random != nil {
			s.sessionSeed = ctx.Random.Seed()
		}
		s.step(ctx.Dt, ctx.Unscaled)
	}
}

func (s *Service) step(game, unscaled float64) {
	s.submitted = 0
	if !s.enabled {
		return
	}
	if s.world.Replaying() {
		if len(s.systems) > 0 {
			s.reset()
		}
		return
	}
	if len(s.systems) == 0 && scene.Store[ParticleSystem2D](s.scene).Len() == 0 {
		s.metrics = Stats{}
		return
	}
	start := time.Now()
	s.serial++
	n := scene.Store[ParticleSystem2D](s.scene).Len()
	if cap(s.completions) < n {
		s.completions = make([]completion, 0, max(n, cap(s.completions)*2))
	}
	scene.Query1[ParticleSystem2D](s.scene).Each(func(h scene.NodeHandle, c *ParticleSystem2D) {
		e := s.state(h, c)
		if !e.valid || !e.pool.Playing() {
			return
		}
		c = &e.cfg
		born, drop := e.pool.Spawned, e.pool.Dropped
		dt := game
		if c.TimeDomain == Unscaled {
			dt = unscaled
		}
		if dt > 0 && e.pool.Step(c, dt, s.scene.RenderWorld(h)) {
			s.completions = append(s.completions, completion{node: h, emitter: e})
		}
		s.pendingBirths += e.pool.Spawned - born
		s.pendingDrops += e.pool.Dropped - drop
	})
	for h, e := range s.systems {
		if e.seen != s.serial {
			s.release(h)
		}
	}
	// Listeners may restart an emitter, destroy a node, or disable the entire world.
	for _, notification := range s.completions {
		if s.enabled && !s.world.Replaying() && s.systems[notification.node] == notification.emitter &&
			s.scene.IsAlive(notification.node) && s.scene.EffectiveEnabled(notification.node) {
			s.module.completed.Emit(s.world, notification.node)
		}
	}
	clear(s.completions)
	s.completions = s.completions[:0]
	if !s.enabled {
		return
	}
	s.metrics.SimulateNS = uint64(time.Since(start).Nanoseconds())
	s.metrics.BornThisFrame, s.metrics.DroppedThisFrame = s.pendingBirths, s.pendingDrops
	s.pendingBirths, s.pendingDrops = 0, 0
}
