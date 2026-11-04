package particles

import (
	"math"
	"slices"
	"testing"
	"unsafe"

	"aego/asset"
	"aego/core/curve"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/render/sprite"
	"aego/schema"
)

func testConfig() ParticleSystem2D {
	c := DefaultParticleSystem2D()
	c.Rate = 0
	c.Lifetime = mathx.V2(10, 10)
	c.Speed = mathx.V2(0, 0)
	c.Size = mathx.V2(1, 1)
	return c
}
func testPool(t testing.TB, n int) *Pool {
	t.Helper()
	p := new(Pool)
	if err := p.Configure(n); err != nil {
		t.Fatal(err)
	}
	p.Reset(12)
	return p
}
func closeNumber(a, b float32) bool { return mathx.Abs(a-b) < 1e-4 }

func TestFractionalRateAndExactBirthAges(t *testing.T) {
	c := testConfig()
	c.Rate = 5
	p := testPool(t, 16)
	p.Play()
	m := mathx.Mat3Identity()
	for range 60 {
		p.Step(&c, 1./60, m)
	}
	if n := len(p.Particles()); n != 5 || p.Requested != 5 || p.Dropped != 0 {
		t.Fatalf("after 60 steps: len=%d requested=%d dropped=%d", n, p.Requested, p.Dropped)
	}
	for i, v := range p.Particles() {
		want := .8 - .2*float32(i)
		if !closeNumber(v.Age, want) {
			t.Fatalf("age %d = %g; want %g", i, v.Age, want)
		}
	}
	c.Rate = 0
	for range 3 {
		p.Step(&c, .1, m)
	}
	if p.Requested != 5 {
		t.Fatalf("zero rate requested births: %d", p.Requested)
	}
}

func TestPoolingWithinAnInterval(t *testing.T) {
	c := testConfig()
	c.Rate = 4
	c.Lifetime = mathx.V2(.375, .375)
	m := mathx.Mat3Identity()
	p := testPool(t, 1)
	p.Burst(&c, 1, m)
	p.Play()
	p.Step(&c, 1, m)
	if p.Spawned != 3 || p.Requested != 5 || p.Dropped != 2 || len(p.Particles()) != 1 || p.Particles()[0].ID != 2 || p.Particles()[0].Age != 0 {
		t.Fatalf("counts %d %d %d, live %+v", p.Requested, p.Spawned, p.Dropped, p.Particles())
	}
}
func TestCycleBurstsAndLifecycle(t *testing.T) {
	c := testConfig()
	c.Duration = .5
	c.Bursts = []mathx.Vec2{{X: 0, Y: 2}, {X: .125, Y: 1}, {X: .125, Y: 3}}
	m := mathx.Mat3Identity()
	p := testPool(t, 40)
	p.Play()
	p.Play()
	p.Step(&c, 1, m)
	if p.Cycle != 2 || p.Spawned != 14 {
		t.Fatalf("2 full cycles plus start: cycle=%d born=%d", p.Cycle, p.Spawned)
	}
	if p.Particles()[0].Age != 1 || p.Particles()[2].Age != .875 || p.Particles()[6].Age != .5 || p.Particles()[12].Age != 0 {
		t.Fatal("burst order or key interval ages are wrong")
	}
	p.Stop(false)
	if p.Emitting() || !p.Playing() {
		t.Fatal("stop must drain live particles")
	}
	if p.Step(&c, 11, m) || p.Playing() {
		t.Fatal("Stop is not Completed")
	}
	p.Burst(&c, 1, m)
	p.Step(&c, .125, m)
	if p.Emitting() || len(p.Particles()) != 1 {
		t.Fatal("manual burst started automatic emission")
	}
	p.Stop(true)
	if p.Playing() {
		t.Fatal("Stop(Clear) did not empty pool")
	}
	c.Loop = false
	c.Lifetime = mathx.V2(.25, .25)
	c.Bursts = []mathx.Vec2{{X: .375, Y: 1}}
	p.Play()
	p.Step(&c, .5, m)
	if p.Emitting() || !p.Playing() || !p.Step(&c, .125, m) || p.Step(&c, 1, m) {
		t.Fatal("completion is not exactly once after natural drain")
	}
	p.Play()
	p.Step(&c, .4, m)
	p.Clear()
	if !p.Emitting() || p.clock == 0 {
		t.Fatal("Clear reset playback")
	}
	p.Stop(true)
	p.Play()
	p.Step(&c, .5, m)
	p.Clear()
	if p.Step(&c, 1, m) {
		t.Fatal("particles explicitly cleared during drain produced Completed")
	}
}
func TestManualBurstOverflowPreservesSurvivors(t *testing.T) {
	c := testConfig()
	m := mathx.Mat3Identity()
	p := testPool(t, 8)
	p.Burst(&c, 3, m)
	before := slices.Clone(p.Particles())
	p.Burst(&c, 1000000000, m)
	if !slices.Equal(p.Particles()[:3], before) || p.Spawned != 8 || p.Dropped != 999999995 {
		t.Fatal("overflow replaced an existing particle or lost its drop count")
	}
	c.Lifetime = mathx.V2(.125, .125)
	p.Clear()
	p.Burst(&c, 2, m)
	c.Lifetime = mathx.V2(10, 10)
	p.Burst(&c, 3, m)
	p.Step(&c, .5, m)
	if len(p.Particles()) != 3 || p.Particles()[0].ID != 10 || p.Particles()[2].ID != 12 {
		t.Fatal("compaction reordered surviving particles")
	}
}

func TestWorldSurfaceFreezesFullBirthBasis(t *testing.T) {
	c := testConfig()
	c.Direction = 0
	c.Spread = 0
	c.Rotation = mathx.V2(0, 0)
	c.Size = mathx.V2(2, 2)
	c.Speed = mathx.V2(1, 1)
	c.Offset = mathx.V2(1, 2)
	birth := mathx.Mat3{-2, .75, 0, 1, 3, 0, 5, 7, 1}
	moved := mathx.Mat3TRS(mathx.V2(95, 14), mathx.Pi/2, mathx.V2(2, 3))
	for _, sp := range []Space{World, Local} {
		c.Space = sp
		p := testPool(t, 4)
		p.Burst(&c, 1, birth)
		p.Step(&c, .5, moved)
		pos, basis, _ := p.Particles()[0].Surface(sp, moved)
		mat := birth
		if sp == Local {
			mat = moved
		}
		want := mat.Apply(mathx.V2(1.5, 2))
		if !closeNumber(pos.X, want.X) || !closeNumber(pos.Y, want.Y) || basis != [4]float32{2 * mat[0], 2 * mat[1], 2 * mat[3], 2 * mat[4]} {
			t.Fatalf("space %d got %v %v, want pos %v", sp, pos, basis, want)
		}
	}
}
func TestShapeDistributionAndOverLife(t *testing.T) {
	c := testConfig()
	c.Lifetime = mathx.V2(1, 1)
	c.Shape = Circle
	c.Radius = 2
	c.Color = mathx.Color{R: 1, G: .4, B: .2, A: .8}
	c.Gravity = mathx.V2(0, 2)
	c.RotationSpeed = 2
	c.SizeOverLife = curve.Line(0, 1, 1, 3)
	p := testPool(t, 4096)
	m := mathx.Mat3Identity()
	p.Burst(&c, 4096, m)
	var r2 float32
	for _, v := range p.Particles() {
		q := v.Position.Dot(v.Position)
		if q > 4.001 {
			t.Fatal("sample outside circle")
		}
		r2 += q
	}
	if q := r2 / 4096; q < 1.9 || q > 2.1 {
		t.Fatalf("expected disk-area samples; E[r²] = %v, want 2", q)
	}
	first := p.Particles()[0].Position
	p.Step(&c, .25, m)
	p.Step(&c, .25, m)
	a := p.Particles()[0]
	if !closeNumber(a.Color.A, .4) || a.Color.R != 1 || !closeNumber(a.Size, 2) || !closeNumber(a.Position.Y-first.Y, .25) || !closeNumber(a.Rotation, 1) {
		t.Fatalf("half-life sample: %+v", a)
	}
	p.Clear()
	c.Shape = Rect
	c.RectSize = mathx.V2(6, 2)
	c.Offset = mathx.V2(.2, .3)
	p.Burst(&c, 1000, m)
	for _, a := range p.Particles() {
		v := a.Position.Sub(c.Offset)
		if mathx.Abs(v.X) > 3 || mathx.Abs(v.Y) > 1 {
			t.Fatal("outside rectangle")
		}
	}
}

func TestSeedIsolationAndWarmAllocations(t *testing.T) {
	c := DefaultParticleSystem2D()
	c.Shape = Circle
	c.Rate = 17
	c.Bursts = []mathx.Vec2{{X: 0, Y: 3}, {X: .5, Y: 2}}
	world := mathx.Mat3Identity()
	a, b, neighbor := testPool(t, 128), testPool(t, 128), testPool(t, 777)
	id := ids.NodeID{Hi: 4, Lo: 9}
	seed := StableSeed(42, id, 1)
	if seed == StableSeed(42, id, 2) || seed == StableSeed(42, ids.NodeID{Hi: 4, Lo: 8}, 1) {
		t.Fatal("derived seed ignores instance/activation")
	}
	a.Reset(seed)
	b.Reset(seed)
	a.Play()
	b.Play()
	for i := 0; i < 300; i++ {
		d := .013 + float64(i%5)*.002
		a.Step(&c, d, world)
		neighbor.Burst(&c, i%13, world)
		neighbor.Step(&c, .03, world)
		b.Step(&c, d, world)
		if i%21 == 0 {
			a.Burst(&c, 6, world)
			b.Burst(&c, 6, world)
		}
		if !slices.Equal(a.Particles(), b.Particles()) || a.Requested != b.Requested {
			t.Fatalf("nondeterministic frame %d", i)
		}
	}
	if got := testing.AllocsPerRun(500, func() {
		a.Burst(&c, 2, world)
		a.Step(&c, 1./60, world)
		for _, x := range a.Particles() {
			x.Surface(c.Space, world)
		}
	}); got != 0 {
		t.Fatalf("%g Go allocations per warmed tick", got)
	}
}
func TestBoundedSchedulingAndValidation(t *testing.T) {
	c := testConfig()
	c.Bursts = []mathx.Vec2{{X: 0, Y: 1}}
	c.Rate = 2
	c.Duration = .5
	c.Lifetime = mathx.V2(100, 100)
	p := testPool(t, 8)
	p.Play()
	for _, dt := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		p.Step(&c, dt, mathx.Mat3Identity())
	}
	if p.Requested != 0 || !p.emitting || p.clock != 0 {
		t.Fatal("invalid dt advanced simulation")
	}
	p.Step(&c, 10000000, mathx.Mat3Identity())
	if p.Cycle != 20000000 || p.Requested != 40000001 || p.Requested != p.Spawned+p.Dropped || len(p.Particles()) != 0 {
		t.Fatalf("large dt %d, %d, %d, %d", p.Cycle, p.Requested, p.Spawned, p.Dropped)
	}
	c.Bursts[0].X = c.Duration
	if c.Validate() == nil {
		t.Fatal("accepted key at cycle end")
	}
	c.Bursts = nil
	c.Gravity.X = float32(math.NaN())
	if c.Validate() == nil {
		t.Fatal("accepted NaN gravity")
	}
	c = testConfig()
	c.MaxParticles = MaxCapacity + 1
	if c.Validate() == nil {
		t.Fatal("accepted capacity over budget")
	}
	c = testConfig()
	c.Layer, c.Order = int32(sprite.LayerMin), int32(sprite.OrderMax)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Layer--
	if c.Validate() == nil {
		t.Fatal("accepted unrepresentable painter layer")
	}
	c.Layer++
	c.Order++
	if c.Validate() == nil {
		t.Fatal("accepted unrepresentable painter order")
	}
}

func BenchmarkSimulate10000(b *testing.B) {
	c := DefaultParticleSystem2D()
	c.Lifetime = mathx.V2(1e7, 1e7)
	c.Rate = 0
	c.Gravity = mathx.V2(.25, 1)
	c.RotationSpeed = .3
	var pools [10]Pool
	m := mathx.Mat3Identity()
	for i := range pools {
		p := &pools[i]
		if err := p.Configure(1000); err != nil {
			b.Fatal(err)
		}
		p.Burst(&c, 1000, m)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range pools {
			pools[j].Step(&c, 1./60, m)
		}
	}
	b.StopTimer()
	if len(pools[9].Particles()) != 1000 {
		b.Fatal("benchmark stopped measuring 10,000 live particles")
	}
}

func BenchmarkSimulateTurnover10000(b *testing.B) {
	c := DefaultParticleSystem2D()
	c.Lifetime = mathx.V2(1, 1)
	c.Rate = 1000
	c.Gravity = mathx.V2(.25, 1)
	var pools [10]Pool
	m := mathx.Mat3Identity()
	var initial uint64
	for i := range pools {
		p := &pools[i]
		if err := p.Configure(1000); err != nil {
			b.Fatal(err)
		}
		p.Play()
		for range 64 {
			p.Step(&c, 1./64, m)
		}
		initial += p.Spawned
		if len(p.items) != 1000 {
			b.Fatal("invalid steady-state setup")
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for j := range pools {
			pools[j].Step(&c, 1./64, m)
		}
	}
	b.StopTimer()
	var born uint64
	var alive int
	for i := range pools {
		born += pools[i].Spawned
		alive += len(pools[i].items)
	}
	born -= initial
	b.ReportMetric(float64(born)/float64(b.N), "births/op")
	b.ReportMetric((float64(born)+float64(10000-alive))/float64(b.N), "deaths/op")
}

func TestEdgeAndPreparedMask(t *testing.T) {
	c := testConfig()
	c.Shape = Edge
	c.EdgeA = mathx.V2(-2, 1)
	c.EdgeB = mathx.V2(2, 3)
	m := mathx.Mat3Identity()
	p := testPool(t, 128)
	p.Burst(&c, 128, m)
	for _, a := range p.Particles() {
		if a.Position.X < -2 || a.Position.X > 2 || !closeNumber(a.Position.Y, 2+a.Position.X*.5) {
			t.Fatalf("off-edge point %+v", a.Position)
		}
	}
	p.Clear()
	c.EdgeB = c.EdgeA
	p.Burst(&c, 2, m)
	if p.items[0].Position != c.EdgeA {
		t.Fatal("zero-length edge did not emit from endpoint")
	}
	p.Reset(17)
	c.Shape = SpriteMask
	p.Play()
	c.Rate = 8
	p.Step(&c, 1, m)
	p.Burst(&c, 4, m)
	if p.Spawned != 0 || p.Dropped != 12 {
		t.Fatalf("empty mask: born %d dropped %d", p.Spawned, p.Dropped)
	}
	points := []mathx.Vec2{{X: -.3, Y: -.5}, {X: .2, Y: .4}}
	c.Offset = mathx.V2(1, 2)
	p.SetMask(points)
	p.Burst(&c, 64, m)
	seen := [2]bool{}
	for _, a := range p.items {
		switch a.Position {
		case points[0].Add(c.Offset):
			seen[0] = true
		case points[1].Add(c.Offset):
			seen[1] = true
		default:
			t.Fatal("sample outside imported frame")
		}
	}
	if !seen[0] || !seen[1] {
		t.Fatal("missing occupied mask cell")
	}
}
func TestGradientVelocityDragAndSimpleBoundaries(t *testing.T) {
	c := testConfig()
	c.Lifetime = mathx.V2(1, 1)
	c.Color = mathx.Color{R: .8, G: .6, B: 1, A: .5}
	c.ColorOverLife = []ColorKey{{Time: 0, Color: mathx.Color{R: 1, A: 1}}, {Time: 1, Color: mathx.Color{B: 1, A: .5}}}
	c.AlphaOverLife = curve.Constant(1)
	c.VelocityOverLife = curve.Constant(.5)
	c.Direction = 0
	c.Spread = 0
	c.Speed = mathx.V2(4, 4)
	p := testPool(t, 8)
	m := mathx.Mat3Identity()
	p.Burst(&c, 1, m)
	for range 5 {
		p.Step(&c, .1, m)
	}
	v := p.items[0]
	if !closeNumber(v.Position.X, 1) || v.Velocity.X != 4 || !closeNumber(v.Color.R, .4) || !closeNumber(v.Color.B, .5) || !closeNumber(v.Color.A, .375) {
		t.Fatalf("age modulation must use start state: %+v", v)
	}
	c.Drag = 4
	p.Step(&c, .125, m)
	if !(p.items[0].Velocity.X < 4 && p.items[0].Velocity.X > 0) {
		t.Fatal("drag accelerated or reversed particle")
	}
	c.PlaneEnabled = true
	c.PlaneNormal = mathx.V2(1, 0)
	c.PlaneDistance = 1
	p.Step(&c, .125, m)
	if len(p.items) != 0 {
		t.Fatal("crossed kill plane")
	}
	c.PlaneEnabled = false
	c.Offset = mathx.V2(2, 3)
	c.KillYEnabled = true
	c.KillY = 2
	p.Burst(&c, 1, m)
	if len(p.items) != 0 {
		t.Fatal("spawn beyond KillY survived")
	}
}
func TestPrewarmPolicyAndNoise(t *testing.T) {
	c := testConfig()
	c.Rate = 12
	c.Prewarm = .5
	c.NoiseStrength = 4
	c.NoiseFrequency = 3
	m := mathx.Mat3Identity()
	a, b := testPool(t, 64), testPool(t, 64)
	a.Play()
	b.Play()
	a.Prewarm(&c, m)
	for range 30 {
		b.Step(&c, 1./60, m)
	}
	if a.Spawned != 6 || b.Spawned != 6 {
		t.Fatal("warmup did not follow schedule")
	}
	for i := range a.items {
		if d := a.items[i].Position.Sub(b.items[i].Position); mathx.Abs(d.X)+mathx.Abs(d.Y) > 1e-6 {
			t.Fatalf("prewarm diverged: %v", d)
		}
	}
	if a.items[0].Position == (mathx.Vec2{}) {
		t.Fatal("noise had no effect")
	}
	a.Reset(47)
	c.ReducedMotion = Reduce
	c.ReducedRate = .25
	a.SetReduced(&c, true)
	for range 8 {
		a.Burst(&c, 1, m)
	}
	a.Play()
	a.Step(&c, 1, m)
	if a.Spawned != 5 {
		t.Fatalf("manual+rate with reduction: got %d, want 5", a.Spawned)
	}
	c.ReducedMotion = Hide
	a.SetReduced(&c, true)
	a.Burst(&c, 1, m)
	a.Step(&c, 1, m)
	if len(a.items) > 0 {
		t.Fatal("hidden effect retained particles")
	}
	a.SetReduced(&c, false)
	a.Step(&c, 1, m)
	if len(a.items) != 12 {
		t.Fatal("motion setting damaged emission schedule")
	}
}
func TestFlipbookAndGradientSchema(t *testing.T) {
	type data struct {
		Frames []asset.Ref[asset.Texture] `aego:"frames,asset=texture"`
		Keys   []ColorKey                 `aego:"colorOverLife"`
	}
	reg := schema.NewRegistry()
	if _, err := schema.RegisterStruct[ColorKey](reg, schema.Name("particles.ColorKey")); err != nil {
		t.Fatal(err)
	}
	d, err := schema.RegisterStruct[data](reg, schema.Name("test.ParticleChannels"))
	if err != nil {
		t.Fatal(err)
	}
	ids2 := [3]ids.AssetID{{Lo: 1}, {Lo: 2}, {Lo: 3}}
	c := testConfig()
	for _, id := range ids2 {
		c.Frames = append(c.Frames, asset.NewRef[asset.Texture](id))
	}
	c.FrameFPS = 4
	c.FrameLoop = true
	if c.FrameIndex(.75) != 0 || c.FrameIndex(.5) != 2 {
		t.Fatal("wrong looping frame")
	}
	c.FrameLoop = false
	if c.FrameIndex(100) != 2 {
		t.Fatal("one-shot did not hold final frame")
	}
	src := data{Frames: c.Frames, Keys: []ColorKey{{Color: mathx.White}, {Time: 1, Color: mathx.Color{R: 1, A: .5}}}}
	var dst data
	for _, name := range []string{"frames", "colorOverLife"} {
		f, ok := d.Field(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		var value schema.Value
		f.Get(unsafe.Pointer(&src), &value)
		if err := f.Set(unsafe.Pointer(&dst), value); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(src.Keys, dst.Keys) || len(dst.Frames) != 3 || dst.Frames[2].ID != ids2[2] {
		t.Fatal("generic schema mutation changed gradient order or frame identity")
	}
}

func BenchmarkSimulateSmoke10000(b *testing.B) {
	c := DefaultParticleSystem2D()
	c.Rate = 0
	c.Lifetime = mathx.V2(100000, 100000)
	c.Drag = .8
	c.NoiseStrength = .32
	c.NoiseFrequency = 3
	c.Gravity = mathx.V2(0, -.7)
	c.SizeOverLife = curve.Line(0, .2, 1, 1.3)
	c.VelocityOverLife = curve.Line(0, 1, 1, .3)
	c.ColorOverLife = []ColorKey{{Time: 0, Color: mathx.White}, {Time: .3, Color: mathx.Color{R: .6, G: .5, B: .4, A: .85}}, {Time: .8, Color: mathx.Color{R: .5, G: .55, B: .6, A: .4}}, {Time: 1, Color: mathx.Color{R: .4, G: .4, B: .4, A: 0}}}
	c.RotationSpeed = .65
	m := mathx.Mat3Identity()
	var pools [10]Pool
	for j := range pools {
		p := &pools[j]
		if err := p.Configure(1000); err != nil {
			b.Fatal(err)
		}
		p.Burst(&c, 1000, m)
		for i := range p.items {
			p.items[i].Age = float32(i) * 90
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for j := range pools {
			pools[j].Step(&c, 1./60, m)
		}
	}
	b.StopTimer()
	if len(pools[9].items) != 1000 {
		b.Fatal("smoke benchmark did not keep 10,000 particles alive")
	}
}
