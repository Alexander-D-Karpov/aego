package particles

import (
	"aego/asset"
	"aego/contracts/signals"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/scene"
	"aego/script/vm"
	"math"
	"testing"
)

type completionHub struct {
	signals.Hub
	fn func(signals.Source)
}

func (*completionHub) Wants(signals.Source) bool                      { return true }
func (*completionHub) Args(int) []vm.Value                            { return nil }
func (h *completionHub) Emit(src signals.Source, _ int, _ []vm.Value) { h.fn(src) }

func TestCompletionCanBurstWithoutDoubleAccounting(t *testing.T) {
	s, h, c := testWorld(t, nil)
	c.Loop = false
	c.Duration = .25
	c.PlayOnStart = true
	c.Lifetime = mathx.V2(.1, .1)
	c.Bursts = []mathx.Vec2{{Y: 1}}
	calls := 0
	s.world.SetService(signals.ServiceID, &completionHub{fn: func(signals.Source) { calls++; s.Burst(h, 3) }})
	s.step(.25, .25)
	st := s.Stats()
	if calls != 1 || st.BornThisFrame != 4 || st.Alive != 3 {
		t.Fatalf("Completed=%d, births=%d (want 4), alive=%d", calls, st.BornThisFrame, st.Alive)
	}
}
func TestCompletionCanDisableWorldWhileTwoEmittersAreFinishing(t *testing.T) {
	s, _, c := testWorld(t, nil)
	c.Loop = false
	c.Duration = .25
	c.PlayOnStart = true
	c.Lifetime = mathx.V2(.1, .1)
	c.Bursts = []mathx.Vec2{{Y: 1}}
	cfg := *c
	other := s.scene.Create(s.scene.Root(), "other")
	*scene.Add[ParticleSystem2D](s.scene, other) = cfg
	s.scene.Flush()
	calls := 0
	s.world.SetService(signals.ServiceID, &completionHub{fn: func(signals.Source) { calls++; s.SetEnabled(false) }})
	s.step(.25, .25)
	if calls != 1 || s.allocated != 0 || s.Stats() != (Stats{}) {
		t.Fatalf("after disabling from Completed: callbacks=%d allocated=%d stats=%+v", calls, s.allocated, s.Stats())
	}
}

type maskBoundary struct {
	asset.Resolver
	threshold uint8
}

func (m *maskBoundary) ParticleMask(_ ids.AssetID, threshold uint8) ([]mathx.Vec2, bool) {
	m.threshold = threshold
	return nil, true
}
func TestMaskThresholdIsMinimumNormalizedAlpha(t *testing.T) {
	c := DefaultParticleSystem2D()
	c.Shape = SpriteMask
	r := new(maskBoundary)
	for n := 0; n <= 255; n++ {
		at := float32(n) / 255
		for _, cut := range []float32{at, math.Nextafter32(at, 1)} {
			c.MaskThreshold = cut
			if _, err := resources(&c, r); err != nil {
				t.Fatal(err)
			}
			for a := 0; a <= 255; a++ {
				got := a != 0 && uint8(a) >= r.threshold
				want := a != 0 && float32(a)/255 >= cut
				if got != want {
					t.Fatalf("alpha=%d/255 cutoff=%g converted to %d: accepted=%v wanted=%v", a, cut, r.threshold, got, want)
				}
			}
		}
	}
}

func TestImmediateKillReleasesSlotForSameTimeBirths(t *testing.T) {
	cfg := testConfig()
	cfg.Shape = Rect
	cfg.RectSize = mathx.V2(0, 1)
	cfg.KillYEnabled = true
	cfg.KillY = 0
	cfg.Lifetime = mathx.V2(.125, .125)
	cfg.Duration = 1
	cfg.Loop = false
	cfg.Bursts = []mathx.Vec2{{Y: 8}}
	pose := mathx.Mat3Identity()
	for seed := uint64(1); seed <= 32; seed++ {
		manual := testPool(t, 1)
		bulk := testPool(t, 1)
		once := testPool(t, 1)
		manual.Reseed(seed)
		bulk.Reseed(seed)
		once.Reseed(seed)
		for range 8 {
			manual.Burst(&cfg, 1, pose)
		}
		bulk.Burst(&cfg, 8, pose)
		once.Play()
		once.Step(&cfg, .25, pose)
		if bulk.Spawned != manual.Spawned || bulk.Dropped != manual.Dropped || len(bulk.items) != len(manual.items) || once.Spawned != manual.Spawned || once.Dropped != manual.Dropped {
			t.Fatalf("seed %d: individual births spawned/dropped/alive=%d/%d/%d, Burst(8)=%d/%d/%d, scheduled=%d/%d", seed, manual.Spawned, manual.Dropped, len(manual.items), bulk.Spawned, bulk.Dropped, len(bulk.items), once.Spawned, once.Dropped)
		}
	}
}

func TestExplicitPlayPrewarmsAnEmptyLoop(t *testing.T) {
	s, h, c := testWorld(t, nil)
	c.Rate = 12
	c.Prewarm = .5
	c.PlayOnStart = false
	c.Loop = true
	s.Play(h)
	s.Play(h)
	if st := s.Stats(); st.Alive != 6 || st.Spawned != 6 {
		t.Fatalf("explicit Play: alive=%d spawned=%d; should prewarm six once", st.Alive, st.Spawned)
	}
	p := NewPreview(nil, nil)
	defer p.Close()
	if err := p.Configure(*c, 17); err != nil {
		t.Fatal(err)
	}
	p.Play()
	p.Play()
	if p.Count() != 6 {
		t.Fatalf("explicit preview Play: %d alive", p.Count())
	}
	// Stopping and playing while old births drain must not age them by prewarm seconds.
	s.Stop(h, false)
	s.Play(h)
	if s.Count(h) != 6 || s.systems[h].pool.Particles()[0].Age >= .5 {
		t.Fatal("Play fast-forwarded living particles")
	}
}
