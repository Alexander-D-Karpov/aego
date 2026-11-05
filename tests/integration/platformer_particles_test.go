package integration

import (
	"crypto/sha256"
	"encoding/binary"
	"os"
	"slices"
	"testing"

	"aego/app/run"
	"aego/contracts/signals"
	"aego/core/rewind"
	"aego/engine"
	"aego/modules/gameplay"
	"aego/modules/particles"
	"aego/scene"
	"aego/script/bind"
	"aego/script/vm"
	"aego/sdk"
)

func observeLandings(t *testing.T, pf *platformer, fn func()) func() {
	return observeMotionEvent(t, pf, "Landed", fn)
}

func observeMotionEvent(t *testing.T, pf *platformer, name string, fn func()) func() {
	t.Helper()
	hub, ok := engine.ServiceOf[signals.Hub](pf.app.Engine.MainWorld(), signals.ServiceID)
	if !ok {
		t.Fatal("missing signal hub")
	}
	td, ok := pf.app.Scene.Registry().ByName(gameplay.NameMotion)
	if ok {
		for i, signal := range td.Signals {
			if signal.Name != name {
				continue
			}
			src := signals.Source{Kind: signals.KindComponent, A: bind.NodeValue(pf.player).A, B: uint64(td.ID)}
			conn := hub.Connect(src, i, func(_ []vm.Value) { fn() })
			if hub.Connected(conn) {
				return func() { hub.Disconnect(conn) }
			}
		}
	}
	t.Fatalf("could not observe Player.Motion2D.%s", name)
	return nil
}

func TestPlatformerLandingDustRoute(t *testing.T) {
	recording, err := os.ReadFile(lightingRouteFile)
	if err != nil {
		t.Fatal(err)
	}
	var referenceHits []checkpointReached
	var digests [lightingRouteFrames]uint64
	var hashes [lightingRouteFrames / 60][32]byte
	for _, enabled := range []bool{false, true} {
		pf := openPlatformer(t)
		w := pf.app.Engine.MainWorld()
		fx := sdk.ParticlesOf(&sdk.Context{World: w})
		fx.SetEnabled(enabled)
		if fx.Enabled() != enabled {
			t.Fatalf("cannot set particle presentation to %v", enabled)
		}
		if _, err := actionsOf(t, pf).Replay(recording); err != nil {
			t.Fatal(err)
		}
		var hits []checkpointReached
		stopCheckpoints := observePlayerCheckpoints(t, pf, &hits)
		landings, jumps, climbs := uint64(0), uint64(0), uint64(0)
		stopLandings := observeLandings(t, pf, func() { landings++ })
		stopJumps := observeMotionEvent(t, pf, "Jumped", func() { jumps++ })
		stopClimbs := observeMotionEvent(t, pf, "Climbed", func() { climbs++ })
		dust := pf.app.Scene.Find(pf.player, "Dust")
		lantern := pf.app.Scene.Find(pf.app.Scene.Root(), "StartLantern")
		embers := pf.app.Scene.Find(lantern, "Embers")
		smoke := pf.app.Scene.Find(lantern, "Smoke")
		if dust.IsZero() || lantern.IsZero() || embers.IsZero() || smoke.IsZero() {
			t.Fatal("incomplete platformer effects")
		}
		hasher := rewind.NewHasher()
		routeHash := sha256.New()
		peak := 0
		for frame := 1; frame <= lightingRouteFrames; frame++ {
			pf.step(1)
			stats := fx.Stats()
			if !enabled && (stats.Spawned != 0 || stats.Alive != 0 || stats.Submitted != 0) {
				t.Fatalf("disabled dust at frame %d: %+v", frame, stats)
			}
			if enabled && (fx.Emitted(dust) != 8*landings+5*jumps+6*climbs || stats.Dropped != 0) {
				t.Fatalf("frame %d: landings=%d jumps=%d climbs=%d emitted=%d, %+v", frame, landings, jumps, climbs, fx.Emitted(dust), stats)
			}
			peak = max(peak, stats.Alive)
			digest := gameplayDigest(pf)
			var b [8]byte
			binary.LittleEndian.PutUint64(b[:], digest)
			routeHash.Write(b[:])
			if !enabled {
				digests[frame-1] = digest
			} else if digest != digests[frame-1] {
				t.Fatalf("dust changed gameplay at frame %d", frame)
			}
			if frame%60 == 0 {
				h, _, _ := run.WorldHash(w, hasher)
				if !enabled {
					hashes[frame/60-1] = h
				} else if h != hashes[frame/60-1] {
					t.Fatalf("dust changed world hash at frame %d: %x != %x", frame, h, hashes[frame/60-1])
				}
			}
		}
		stopLandings()
		stopJumps()
		stopClimbs()
		stopCheckpoints()
		pf.noFaults(t)
		finish := pf.app.Scene.Find(pf.app.Scene.Root(), "Finish")
		if len(hits) < 2 || hits[len(hits)-1].point != pf.app.Scene.ID(finish) || hits[len(hits)-1].tick != lightingFinishTick {
			t.Fatalf("particles=%v: missing expected Finish: %+v", enabled, hits)
		}
		if !enabled {
			referenceHits = hits
		} else if !slices.Equal(referenceHits, hits) || landings < 2 || peak == 0 || fx.Emitted(smoke) == 0 || fx.Emitted(embers) == 0 {
			t.Fatalf("checkpoint or dust event mismatch: %+v; landings=%d peak=%d", hits, landings, peak)
		}
		t.Logf("particles=%v frames=%d finish=%d landings=%d jumps=%d climbs=%d dust=%d sparks=%d smoke=%d spawned=%d peak=%d gameplayRouteSHA=%x world=%x",
			enabled, lightingRouteFrames, hits[len(hits)-1].tick, landings, jumps, climbs, fx.Emitted(dust), fx.Emitted(embers), fx.Emitted(smoke), fx.Stats().Spawned, peak, routeHash.Sum(nil), hashes[len(hashes)-1])
	}
}

func TestTopdownFootstepDust(t *testing.T) {
	td := openTopdown(t)
	w := td.app.Engine.MainWorld()
	fx := particles.Of(w)
	dust := td.app.Scene.Find(td.hero, "StepDust")
	if _, ok := scene.Get[particles.ParticleSystem2D](td.app.Scene, dust); !ok {
		t.Fatal("missing footstep emitter")
	}
	for range 30 {
		td.step()
	}
	if n := fx.Emitted(dust); n != 0 {
		t.Fatalf("idle hero emitted %d particles", n)
	}
	td.set(0, "MoveX", 1)
	for range 30 {
		td.step()
	}
	if n := fx.Emitted(dust); n == 0 || n%2 != 0 {
		t.Fatalf("running hero emitted %d particles", n)
	}
	if faults := td.app.Scripts.Faults(); len(faults) != 0 {
		t.Fatal(faults)
	}
}
