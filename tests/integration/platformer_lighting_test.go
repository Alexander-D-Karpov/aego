package integration

import (
	"encoding/binary"
	"flag"
	"hash/fnv"
	"image/png"
	"math"
	"os"
	"slices"
	"testing"

	"aego/asset"
	"aego/contracts/signals"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine"
	"aego/modules/gameplay"
	"aego/modules/input"
	"aego/modules/physics/components"
	"aego/modules/render2d"
	"aego/render/lighting"
	"aego/scene"
	"aego/script/bind"
	"aego/script/vm"
)

var updateLightingRoute = flag.Bool("update-lighting-route", false, "record the complete platformer lighting route through Finish")

const lightingRouteFile = "testdata/platformer_lighting_route.aeinput"
const lightingRouteFrames = 2400
const lightingFinishTick = 1810

// gameplayDigest excludes presentation-only lighting nodes and rendering
// caches. It compares actual actors, physics, health and player gameplay state.
func gameplayDigest(pf *platformer) uint64 {
	h := fnv.New64a()
	s := pf.app.Scene
	var buf [8]byte
	put := func(v uint64) { binary.LittleEndian.PutUint64(buf[:], v); _, _ = h.Write(buf[:]) }
	f32 := func(v float32) { put(uint64(math.Float32bits(v))) }
	boolean := func(v bool) {
		if v {
			put(1)
		} else {
			put(0)
		}
	}
	put(pf.app.Engine.MainWorld().Random.State())
	for _, n := range s.PreOrder() {
		if !s.IsAlive(n) {
			continue
		}
		b, body := scene.Get[components.Body2D](s, n)
		hp, health := scene.Get[gameplay.Health](s, n)
		if !body && !health {
			continue
		}
		id := s.ID(n)
		put(id.Hi)
		put(id.Lo)
		p := s.Local(n)
		f32(p.Position.X)
		f32(p.Position.Y)
		f32(p.Rotation)
		if body {
			f32(b.Velocity.X)
			f32(b.Velocity.Y)
			f32(b.AngularVelocity)
		}
		if health {
			put(uint64(hp.Current))
			boolean(hp.Dead)
			boolean(hp.Invulnerable)
		}
	}
	m := pf.motion()
	f32(m.Velocity.X)
	f32(m.Velocity.Y)
	boolean(m.Grounded)
	boolean(m.OnWall)
	boolean(m.OnCeiling)
	boolean(m.Climbing)
	f32(m.FloorNormal.X)
	f32(m.FloorNormal.Y)
	put(uint64(pf.field("kills").Int()))
	stats := gameplay.Of(pf.app.Engine.MainWorld()).Stats()
	put(uint64(stats.Deaths))
	put(uint64(stats.Respawns))
	return h.Sum64()
}

// First area keeps the regression route; continuation crosses existing platforms via Input Actions: no teleports, collider edits, speed/health changes or gameplay RNG use.
func recordLightingRoute(t *testing.T) []byte {
	t.Helper()
	pf := openPlatformer(t)
	in := actionsOf(t, pf)
	in.Record(input.RecordMeta{Project: "platformer", Seed: 42, FixedHz: 60})
	drivePlatformer(t, pf, runFrames)
	waypoints := []struct{ x, y, launch float32 }{
		{20.5, .55, 14}, {25.5, -1.45, 20.5}, {34.5, 1, 26.3},
		{43.5, -.15, 39}, {47.5, -1.15, 43.5}, {51.5, .15, 47.5},
		{56.5, 3, 51.5}, {66.6, 3, 56.5},
	}
	phase, jumpTicks := 0, 0
	var move, jump, attack float32
	set := func(name string, value float32, previous *float32) {
		if value != *previous {
			*previous = value
			if err := in.Inject(0, name, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	for frame := runFrames; frame < lightingRouteFrames; frame++ {
		p := pf.app.Scene.WorldPosition(pf.player)
		m := pf.motion()
		if phase < len(waypoints) {
			g := waypoints[phase]
			tolerance := float32(.5)
			if phase == len(waypoints)-1 {
				tolerance = .1
			}
			if mathx.Abs(p.X-g.x) < tolerance && m.Grounded && p.Y <= g.y+.13 {
				phase++
				t.Logf("lighting route checkpoint %d at frame %d: %.3f, %.3f", phase, frame, p.X, p.Y)
			}
		}
		if phase == len(waypoints) {
			set("Move", 0, &move)
			set("Jump", 0, &jump)
			set("Attack", 0, &attack)
			pf.step(1)
			continue
		}
		g := waypoints[phase]
		desired := mathx.Clamp((g.x-p.X)*1.6, -1, 1)
		set("Move", desired, &move)
		if jumpTicks > 0 {
			jumpTicks--
			if jumpTicks == 0 {
				set("Jump", 0, &jump)
			}
		} else if m.Grounded && p.X >= g.launch-.15 && (phase < len(waypoints)-1 || g.y < p.Y-.2) {
			set("Jump", 1, &jump)
			jumpTicks = 43
		}
		if (frame-runFrames)%28 == 0 {
			set("Attack", 1, &attack)
		} else {
			set("Attack", 0, &attack)
		}
		pf.step(1)
		if frame%120 == 0 {
			t.Logf("route frame %d phase %d at %.3f, %.3f grounded=%v", frame, phase, p.X, p.Y, m.Grounded)
		}
	}
	pf.noFaults(t)
	if p := pf.app.Scene.WorldPosition(pf.player); p.X < 66.5 || phase != len(waypoints) {
		t.Fatalf("full lighting route did not reach Finish: phase=%d position=%v stats=%+v", phase, p, gameplay.Of(pf.app.Engine.MainWorld()).Stats())
	}
	t.Logf("full route gameplay stats: %+v", gameplay.Of(pf.app.Engine.MainWorld()).Stats())
	return in.StopRecording()
}

type checkpointReached struct {
	point, by ids.NodeID
	tick      uint64
}

// Observe the actual authored Checkpoint2D.Reached signal. A waypoint or an
// X-coordinate alone does not prove that Respawn took this checkpoint.
func observePlayerCheckpoints(t *testing.T, pf *platformer, hits *[]checkpointReached) func() {
	t.Helper()
	s, w := pf.app.Scene, pf.app.Engine.MainWorld()
	hub, ok := engine.ServiceOf[signals.Hub](w, signals.ServiceID)
	if !ok {
		t.Fatal("checkpoint signal hub is missing")
	}
	td, ok := s.Registry().ByName(gameplay.NameCheckpoint)
	if !ok {
		t.Fatal("Checkpoint2D is not registered")
	}
	index := -1
	for i, d := range td.Signals {
		if d.Name == "Reached" {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("Checkpoint2D.Reached is not registered")
	}
	var conns []vm.Value
	scene.Query1[gameplay.Checkpoint2D](s).Each(func(h scene.NodeHandle, _ *gameplay.Checkpoint2D) {
		id := s.ID(h)
		src := signals.Source{Kind: signals.KindComponent, A: bind.NodeValue(h).A, B: uint64(td.ID)}
		conn := hub.Connect(src, index, func(args []vm.Value) {
			if len(args) != 1 {
				t.Fatal("Reached must carry the activating node")
			}
			if by := bind.NodeOf(args[0]); by == pf.player {
				*hits = append(*hits, checkpointReached{id, s.ID(by), w.Time.Tick()})
			}
		})
		if !hub.Connected(conn) {
			t.Fatalf("could not observe %s.Reached", s.Name(h))
		}
		conns = append(conns, conn)
	})
	return func() {
		for _, conn := range conns {
			hub.Disconnect(conn)
		}
	}
}

func TestPlatformerLightingRoutePreservesGameplay(t *testing.T) {
	if *updateLightingRoute {
		data := recordLightingRoute(t)
		if err := os.WriteFile(lightingRouteFile, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(lightingRouteFile)
	if err != nil {
		t.Fatalf("%v; record with go test ./tests/integration -run TestPlatformerLightingRoutePreservesGameplay -update-lighting-route", err)
	}
	var reference []uint64
	var referenceHits []checkpointReached
	for _, enabled := range []bool{true, false} {
		pf := openPlatformer(t)
		pf.app.Runner.SetLightingView(enabled, lighting.DebugLighting)
		var hits []checkpointReached
		stop := observePlayerCheckpoints(t, pf, &hits)
		if _, err := actionsOf(t, pf).Replay(data); err != nil {
			t.Fatal(err)
		}
		states := make([]uint64, 0, lightingRouteFrames/60)
		for frame := 1; frame <= lightingRouteFrames; frame++ {
			pf.step(1)
			if frame%60 == 0 {
				states = append(states, gameplayDigest(pf))
			}
		}
		stop()
		s := pf.app.Scene
		finish := s.Find(s.Root(), "Finish")
		cp, ok := scene.Get[gameplay.Checkpoint2D](s, finish)
		if !ok || len(hits) < 2 || hits[len(hits)-1].point != s.ID(finish) {
			t.Fatalf("lighting=%v did not actually activate intermediate checkpoints and Finish: %+v", enabled, hits)
		}
		if p := pf.app.Scene.WorldPosition(pf.player); p.X < 66.5 || !pf.motion().Grounded {
			t.Fatalf("lighting enabled=%v route missed Finish: %v", enabled, p)
		}
		if stats := gameplay.Of(pf.app.Engine.MainWorld()).Stats(); stats.Respawns != 0 {
			t.Fatalf("lighting route respawned with enabled=%v: %+v", enabled, stats)
		}
		pf.noFaults(t)
		if enabled {
			reference = states
			referenceHits = hits
		} else {
			if !slices.Equal(referenceHits, hits) {
				t.Fatalf("lighting changed the Reached emitters, activating node or fixed ticks: %v != %v", referenceHits, hits)
			}
			for i, v := range states {
				if v != reference[i] {
					t.Fatalf("lighting changed gameplay at fixed tick %d: %x != %x", (i+1)*60, v, reference[i])
				}
			}
		}
		// Separately, after the complete unmodified replay, exercise the stock
		// respawn behavior. Sample the return before gravity settles the player.
		want := s.WorldPosition(finish)
		if spawn, ok := cp.SpawnPoint.Resolve(s); ok {
			want = s.WorldPosition(spawn)
		}
		resp, ok := scene.Get[gameplay.Respawn](s, pf.player)
		if !ok {
			t.Fatal("player has no Respawn component")
		}
		resp.Trigger()
		returned := false
		for step := 0; step < 120; step++ {
			pf.step(1)
			p := s.WorldPosition(pf.player)
			if gameplay.Of(pf.app.Engine.MainWorld()).Stats().Respawns == 1 && mathx.Abs(p.X-want.X) < .001 && mathx.Abs(p.Y-want.Y) < .1 {
				returned = true
				break
			}
		}
		if !returned {
			t.Fatalf("lighting=%v: actual respawn never returned to Finish spawn %v; player %v", enabled, want, s.WorldPosition(pf.player))
		}
		pf.noFaults(t)
	}
}

func TestPlatformerRunnerNormalsFollowAnimation(t *testing.T) {
	pf := openPlatformer(t)
	source, ok := any(pf.app.Loader).(asset.NormalSource)
	if !ok {
		t.Fatal("runtime loader exposes no prepared normal associations")
	}
	file, err := os.Open(pf.app.Pipeline.Abs("assets/surfaces/runner.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	image, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	// The project sheet retains exactly the eight original 16×16 silhouettes;
	// every animation frame has a separate normal reference and original canvas.
	for i := 0; i < 8; i++ {
		original, exists := asset.BuiltinTexture(asset.BuiltinRunner(i))
		if !exists {
			t.Fatal("original runner source missing")
		}
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				r, g, b, a := image.At(i*16+x, y).RGBA()
				want := original.Pixels[(y*16+x)*4:][:4]
				if byte(r>>8) != want[0] || byte(g>>8) != want[1] || byte(b>>8) != want[2] || byte(a>>8) != want[3] {
					t.Fatalf("runner frame %d changed original pixel %d,%d", i, x, y)
				}
			}
		}
		id, found := pf.app.Pipeline.Lookup("assets/surfaces/runner.png#runner_00" + string(rune('0'+i)))
		if !found {
			t.Fatal("missing local runner frame", i)
		}
		albedo, ok := pf.app.Loader.Texture(id)
		normalID, normal, resolved := source.NormalFor(id)
		if !ok || !resolved || normalID.IsZero() || normalID == id || albedo.Size() != normal.Size() {
			t.Fatalf("runner frame %d normal mismatch: albedo=%+v normal=%+v", i, albedo, normal)
		}
	}
}

func TestPlatformerLightingControlsChangeViewOnly(t *testing.T) {
	pf := openPlatformer(t)
	pf.step(10)
	materials := map[scene.NodeHandle]ids.AssetID{}
	scene.Query1[render2d.SpriteRenderer](pf.app.Scene).Each(func(h scene.NodeHandle, s *render2d.SpriteRenderer) { materials[h] = s.Material.ID })
	press := func(action string) {
		if err := actionsOf(t, pf).Inject(0, action, 1); err != nil {
			t.Fatal(err)
		}
		pf.step(1)
		if err := actionsOf(t, pf).Inject(0, action, 0); err != nil {
			t.Fatal(err)
		}
		pf.step(1)
	}
	press("LightingToggle")
	if enabled, _ := pf.app.Runner.LightingView(); enabled {
		t.Fatal("registered F6 action did not disable lighting view")
	}
	press("LightingDebug")
	if _, debug := pf.app.Runner.LightingView(); debug != lighting.DebugNormals {
		t.Fatal("registered F7 action did not enter normal preview")
	}
	press("LightingDebug")
	if _, debug := pf.app.Runner.LightingView(); debug != lighting.DebugShadows {
		t.Fatal("registered F7 action did not enter shadow preview")
	}
	press("LightingDebug")
	press("LightingToggle")
	if enabled, debug := pf.app.Runner.LightingView(); !enabled || debug != lighting.DebugLighting {
		t.Fatal("lighting controls did not restore the normal view")
	}
	for h, id := range materials {
		s, ok := scene.Get[render2d.SpriteRenderer](pf.app.Scene, h)
		if !ok || s.Material.ID != id {
			t.Fatal("lighting view toggle changed a scene material")
		}
	}
	pf.noFaults(t)
}
