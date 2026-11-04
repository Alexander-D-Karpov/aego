package particles

import (
	"errors"
	"math"

	"aego/core/curve"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/core/rng"
)

type Particle struct {
	Position, Velocity            mathx.Vec2
	Age, Lifetime, Size, Rotation float32
	Color                         mathx.Color
	Basis                         [4]float32
	ID                            uint64
	startSize                     float32
	startColor                    mathx.Color
	noiseID                       uint32
}

func (p Particle) Surface(space Space, pose mathx.Mat3) (mathx.Vec2, [4]float32, mathx.Color) {
	pos, b := p.Position, p.Basis
	if space == Local {
		pos = pose.Apply(pos)
		b = linear(pose)
	}
	c, s := p.Size, float32(0)
	if p.Rotation != 0 {
		sn, cs := math.Sincos(float64(p.Rotation))
		c, s = p.Size*float32(cs), p.Size*float32(sn)
	}
	return pos, [4]float32{b[0]*c + b[2]*s, b[1]*c + b[3]*s, b[2]*c - b[0]*s, b[3]*c - b[1]*s}, p.Color
}
func linear(m mathx.Mat3) [4]float32 { return [4]float32{m[0], m[1], m[3], m[4]} }

type Pool struct {
	items                       []Particle
	deaths                      []float64
	source                      rng.Source
	mask                        []mathx.Vec2
	reduced                     bool
	budgetCarry                 float64
	clock, carry                float64
	emitting, draining, atStart bool
	nextID, Cycle               uint64
	Requested, Spawned, Dropped uint64
}

func (p *Pool) Configure(capacity int) error {
	if capacity < 1 || capacity > MaxCapacity {
		return errors.New("invalid particle pool capacity")
	}
	if cap(p.items) != capacity {
		p.items = make([]Particle, 0, capacity)
		p.deaths = make([]float64, 0, capacity)
		p.Reset(1)
	}
	return nil
}
func (p *Pool) Reset(seed uint64) {
	*p = Pool{items: p.items[:0], deaths: p.deaths[:0]}
	p.Reseed(seed)
}
func (p *Pool) Reseed(seed uint64) { p.source.SetState(seed) }
func (p *Pool) Play() {
	if p.emitting {
		return
	}
	p.emitting, p.draining, p.atStart = true, false, true
	p.clock, p.carry, p.Cycle = 0, 0, 0
}
func (p *Pool) Stop(clear bool) {
	p.emitting, p.draining, p.atStart = false, false, false
	if clear {
		p.Clear()
	}
}
func (p *Pool) Clear() {
	p.items = p.items[:0]
	p.draining = false
}
func (p *Pool) Emitting() bool        { return p.emitting }
func (p *Pool) Playing() bool         { return p.emitting || p.draining || len(p.items) > 0 }
func (p *Pool) Particles() []Particle { return p.items }

func StableSeed(session uint64, id ids.NodeID, activation uint64) uint64 {
	var r rng.Source
	r.SetState(session)
	r.SetState(r.Uint64() ^ id.Hi)
	r.SetState(r.Uint64() ^ id.Lo)
	r.SetState(r.Uint64() ^ activation)
	return r.Uint64()
}

func rangeValue(v mathx.Vec2, r *rng.Source) float32 {
	if v.X == v.Y {
		return v.X
	}
	return r.Range(v.X, v.Y)
}
func over(c *curve.Curve, t float32) float32 {
	if len(c.Keys) == 1 {
		return c.Keys[0].V
	}
	return c.EvalClamped(t)
}
func appearance(a *Particle, c *ParticleSystem2D) {
	u := mathx.Clamp(a.Age/a.Lifetime, 0, 1)
	a.Size = a.startSize * max(0, over(&c.SizeOverLife, u))
	a.Color = a.startColor
	if len(c.ColorOverLife) > 0 {
		g := GradientAt(c.ColorOverLife, u)
		a.Color.R *= g.R
		a.Color.G *= g.G
		a.Color.B *= g.B
		a.Color.A *= g.A
	}
	a.Color.A *= mathx.Clamp(over(&c.AlphaOverLife, u), 0, 1)
}
func noiseValue(x uint32) float32 {
	x ^= x >> 16
	x *= 0x7feb352d
	x ^= x >> 15
	x *= 0x846ca68b
	x ^= x >> 16
	return float32(x>>8)*(2./(1<<24)) - 1
}
func noiseAt(key uint32, t float64) float32 {
	i := math.Floor(t)
	f := float32(t - i)
	f = f * f * (3 - 2*f)
	n := uint32(math.Mod(i, 1<<32))
	return mathx.Lerp(noiseValue(key+n), noiseValue(key+n+1), f)
}
func advance(a *Particle, c *ParticleSystem2D, dt float32) {
	g := c.Gravity
	if c.NoiseStrength > 0 {
		t := float64(c.NoiseFrequency) * (float64(a.Age) + float64(dt)*.5)
		g.X += c.NoiseStrength * noiseAt(a.noiseID, t)
		g.Y += c.NoiseStrength * noiseAt(a.noiseID^0xa341316c, t)
	}
	v := a.Velocity
	a.Velocity.X += g.X * dt
	a.Velocity.Y += g.Y * dt
	if c.Drag > 0 {
		drag := 1 / (1 + c.Drag*dt)
		a.Velocity.X *= drag
		a.Velocity.Y *= drag
	}
	scale := dt * 0.5 * over(&c.VelocityOverLife, mathx.Clamp((a.Age+dt*.5)/a.Lifetime, 0, 1))
	a.Position.X += (v.X + a.Velocity.X) * scale
	a.Position.Y += (v.Y + a.Velocity.Y) * scale
	a.Age += dt
	a.Rotation += c.RotationSpeed * dt
	appearance(a, c)
}
func killed(c *ParticleSystem2D, pos mathx.Vec2) bool {
	if c.KillYEnabled && pos.Y > c.KillY {
		return true
	}
	if !c.PlaneEnabled {
		return false
	}
	nx, ny := float64(c.PlaneNormal.X), float64(c.PlaneNormal.Y)
	return float64(pos.X)*nx+float64(pos.Y)*ny > float64(c.PlaneDistance)*math.Hypot(nx, ny)
}
func (p *Pool) newborn(c *ParticleSystem2D, pose mathx.Mat3) Particle {
	pos := c.Offset
	switch c.Shape {
	case Circle:
		r := c.Radius * mathx.Sqrt(p.source.Float32())
		a := float64(p.source.Float32()) * 2 * math.Pi
		s, co := math.Sincos(a)
		pos.X += r * float32(co)
		pos.Y += r * float32(s)
	case Edge:
		pos = pos.Add(c.EdgeA.Lerp(c.EdgeB, p.source.Float32()))
	case SpriteMask:
		pos = pos.Add(p.mask[p.source.IntN(len(p.mask))])
	case Rect:
		pos.X += (p.source.Float32() - .5) * c.RectSize.X
		pos.Y += (p.source.Float32() - .5) * c.RectSize.Y
	}
	speed := rangeValue(c.Speed, &p.source)
	angle := c.Direction
	if c.Spread > 0 {
		angle += (p.source.Float32() - .5) * c.Spread
	}
	var v mathx.Vec2
	if speed != 0 {
		sn, cs := math.Sincos(float64(angle))
		v = mathx.V2(speed*float32(cs), speed*float32(sn))
	}
	a := Particle{Position: pos, Velocity: v, Lifetime: rangeValue(c.Lifetime, &p.source), startSize: rangeValue(c.Size, &p.source), Rotation: rangeValue(c.Rotation, &p.source), Color: c.Color, startColor: c.Color, ID: p.nextID}
	a.noiseID = uint32(p.source.State()) ^ uint32(a.ID)
	p.nextID++
	if c.Space == World {
		a.Position = pose.Apply(pos)
		a.Velocity = pose.ApplyVector(v)
		a.Basis = linear(pose)
	}
	return a
}

func add(n *uint64, v uint64) { *n += min(v, ^uint64(0)-*n) }
func count(x float64) uint64 {
	if x >= float64(^uint64(0)) {
		return ^uint64(0)
	}
	if !(x > 0) {
		return 0
	}
	return uint64(x)
}

func (p *Pool) Burst(c *ParticleSystem2D, n int, pose mathx.Mat3) {
	if n <= 0 {
		return
	}
	n = int(p.quota(c, uint64(n)))
	add(&p.Requested, uint64(n))
	attempts := min(n, maxAttemptsPerStep)
	if c.Shape == SpriteMask && len(p.mask) == 0 {
		attempts = 0
	}
	accepted := 0
	for accepted < attempts && len(p.items) < cap(p.items) {
		a := p.newborn(c, pose)
		accepted++
		if killed(c, a.Position) {
			continue
		}
		appearance(&a, c)
		p.items = append(p.items, a)
	}
	add(&p.Spawned, uint64(accepted))
	add(&p.Dropped, uint64(n-accepted))
}

// Particles that die inside this step still occupy a slot until their precise
// death time. Keep only those times; survivors stay in stable birth order.
func (p *Pool) expires(t float64) {
	ds := append(p.deaths, t)
	i := len(ds) - 1
	for i > 0 {
		j := (i - 1) / 2
		if ds[j] <= t {
			break
		}
		ds[i] = ds[j]
		i = j
	}
	ds[i] = t
	p.deaths = ds
}
func (p *Pool) release(t float64) {
	for len(p.deaths) > 0 && p.deaths[0] <= t {
		ds := p.deaths
		last := len(ds) - 1
		v := ds[last]
		ds = ds[:last]
		if last > 0 {
			i := 0
			for j := 1; j < last; j = i*2 + 1 {
				if j+1 < last && ds[j+1] < ds[j] {
					j++
				}
				if ds[j] >= v {
					break
				}
				ds[i] = ds[j]
				i = j
			}
			ds[i] = v
		}
		p.deaths = ds
	}
}

const maxCyclesPerStep = 64
const maxAttemptsPerStep = MaxCapacity

type pass struct {
	p        *Pool
	c        *ParticleSystem2D
	pose     mathx.Mat3
	end      float64
	attempts int
}

func (s *pass) emit(n uint64, first, spacing float64) {
	p := s.p
	add(&p.Requested, n)
	if s.c.Shape == SpriteMask && len(p.mask) == 0 {
		add(&p.Dropped, n)
		return
	}
	tries := int(min(n, uint64(s.attempts)))
	s.attempts -= tries
	accepted := uint64(0)
	for k := 0; k < tries; k++ {
		t := min(s.end, first+float64(k)*spacing)
		p.release(t)
		if len(p.items)+len(p.deaths) >= cap(p.items) {
			if spacing == 0 || len(p.deaths) == 0 {
				break
			}
			continue
		}
		a := p.newborn(s.c, s.pose)
		accepted++
		if killed(s.c, a.Position) {
			continue
		}
		if t+float64(a.Lifetime) <= s.end {
			p.expires(t + float64(a.Lifetime))
			continue
		}
		advance(&a, s.c, float32(s.end-t))
		if killed(s.c, a.Position) {
			p.expires(s.end)
			continue
		}
		p.items = append(p.items, a)
	}
	add(&p.Spawned, accepted)
	add(&p.Dropped, n-accepted)
}
func (s *pass) rate(start, dt float64) {
	if dt <= 0 || s.c.Rate <= 0 {
		return
	}
	rate := float64(s.c.Rate) * s.p.factor(s.c)
	if rate <= 0 {
		return
	}
	sum := s.p.carry + rate*dt
	n := math.Floor(sum + 1e-12)
	first := start + (1-s.p.carry)/rate
	s.p.carry = max(0, sum-n)
	if n >= 1 {
		s.emit(count(n), first, 1/rate)
	}
}
func (s *pass) startKeys(t float64) {
	for _, key := range s.c.Bursts {
		if key.X != 0 {
			break
		}
		s.emit(s.p.quota(s.c, count(float64(key.Y))), t, 0)
	}
}
func (s *pass) segment(start, from, to float64) {
	last := from
	for _, key := range s.c.Bursts {
		at := float64(key.X)
		if at <= from {
			continue
		}
		if at > to {
			break
		}
		s.rate(start+(last-from), at-last)
		s.emit(s.p.quota(s.c, count(float64(key.Y))), start+(at-from), 0)
		last = at
	}
	s.rate(start+(last-from), to-last)
}
func (s *pass) skip(remaining float64) {
	p, c := s.p, s.c
	d := float64(c.Duration)
	cycles := math.Floor(remaining / d)
	tail := math.Mod(remaining, d)
	var all, part float64
	for _, key := range c.Bursts {
		all += float64(key.Y)
		if key.X > 0 && float64(key.X) <= tail {
			part += float64(key.Y)
		}
	}
	q := p.carry + remaining*float64(c.Rate)*p.factor(c)
	n := math.Floor(q + 1e-12)
	p.carry = max(0, q-n)
	discarded := p.quota(c, count(all*cycles+part))
	add(&discarded, count(n))
	add(&p.Requested, discarded)
	add(&p.Dropped, discarded)
	add(&p.Cycle, count(cycles))
	p.clock = tail
}
func (p *Pool) Step(c *ParticleSystem2D, dt float64, pose mathx.Mat3) bool {
	if !(dt > 0) || math.IsInf(dt, 0) {
		return false
	}
	dt = min(dt, float64(math.MaxFloat32))
	p.deaths = p.deaths[:0]
	write := 0
	for i := range p.items {
		a := &p.items[i]
		life := float64(a.Lifetime - a.Age)
		if life <= dt {
			if p.emitting {
				p.expires(life)
			}
			continue
		}
		advance(a, c, float32(dt))
		if killed(c, a.Position) {
			if p.emitting {
				p.expires(dt)
			}
			continue
		}
		if i != write {
			p.items[write] = *a
		}
		write++
	}
	p.items = p.items[:write]
	if p.emitting {
		d := float64(c.Duration)
		if !(d > 0) || math.IsInf(d, 0) {
			p.Stop(false)
			return false
		}
		s := pass{p: p, c: c, pose: pose, end: dt, attempts: maxAttemptsPerStep}
		if p.atStart {
			s.startKeys(0)
			p.atStart = false
		}
		offset := 0.0
		for i := 0; offset < dt && p.emitting; i++ {
			from := min(p.clock, d)
			span := min(dt-offset, d-from)
			s.segment(offset, from, from+span)
			p.clock = from + span
			offset += span
			if p.clock < d {
				break
			}
			if !c.Loop {
				p.emitting = false
				p.draining = true
				break
			}
			add(&p.Cycle, 1)
			p.clock = 0
			s.startKeys(offset)
			if i+1 >= maxCyclesPerStep && offset < dt {
				s.skip(dt - offset)
				break
			}
		}
	}
	if p.draining && len(p.items) == 0 {
		p.draining = false
		return true
	}
	return false
}

// SetMask borrows immutable prepared positions in the sprite frame, expressed relative to its pivot.
func (p *Pool) SetMask(points []mathx.Vec2) { p.mask = points }
func (p *Pool) SetReduced(c *ParticleSystem2D, on bool) {
	if p.reduced != on {
		p.budgetCarry = 0
	}
	p.reduced = on
	if on && c.ReducedMotion == Hide {
		p.Clear()
	}
}
func (p *Pool) factor(c *ParticleSystem2D) float64 {
	if !p.reduced || c.ReducedMotion == Keep {
		return 1
	}
	if c.ReducedMotion == Hide {
		return 0
	}
	return float64(c.ReducedRate)
}
func (p *Pool) quota(c *ParticleSystem2D, n uint64) uint64 {
	f := p.factor(c)
	if f == 1 {
		return n
	}
	if f == 0 {
		return 0
	}
	x := p.budgetCarry + float64(n)*f
	a := math.Floor(x + 1e-12)
	p.budgetCarry = max(0, x-a)
	return count(a)
}
func (p *Pool) Prewarm(c *ParticleSystem2D, pose mathx.Mat3) {
	if !c.Loop || !p.emitting || !(c.Prewarm > 0) {
		return
	}
	t := min(float64(c.Prewarm), 2)
	for i := 0; t > 0 && i < 120; i++ {
		dt := min(t, 1./60)
		p.Step(c, dt, pose)
		t -= dt
	}
}
