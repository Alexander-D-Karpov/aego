package clock

import "math"

type Domain uint8

const (
	Real Domain = iota
	Game
	Fixed
	Editor
	Audio
	DomainCount
)

const MaxScale = 8

type Span struct {
	Delta   float64
	Elapsed float64
	Scale   float64
	Paused  bool
}

type Config struct {
	FixedDelta    float64
	MaxFrameDelta float64
	MaxFixedSteps int
}

func (c Config) WithDefaults() Config {
	if c.FixedDelta <= 0 {
		c.FixedDelta = DefaultFixedDelta
	}
	if c.MaxFrameDelta <= 0 {
		c.MaxFrameDelta = DefaultMaxFrameDelta
	}
	if c.MaxFixedSteps <= 0 {
		c.MaxFixedSteps = DefaultMaxSteps
	}
	return c
}

type Plan struct {
	Steps  int
	Update bool
	Paused bool
}

type step uint8

const (
	stepNone step = iota
	stepFixed
	stepFrame
)

type Time struct {
	cfg      Config
	spans    [DomainCount]Span
	scale    float64
	paused   bool
	pending  step
	acc      float64
	alpha    float64
	frame    uint64
	tick     uint64
	dropped  uint64
	clamped  float64
	lastGame float64
}

func NewTime(cfg Config) *Time {
	t := &Time{scale: 1}
	t.Configure(cfg)
	return t
}

func (t *Time) Configure(cfg Config) {
	t.cfg = cfg.WithDefaults()
}

func (t *Time) Config() Config {
	return t.cfg
}

func (t *Time) FixedDelta() float64 {
	return t.cfg.FixedDelta
}

func (t *Time) Span(d Domain) Span {
	if d >= DomainCount {
		return Span{}
	}
	return t.spans[d]
}

func (t *Time) Frame() uint64 {
	return t.frame
}

func (t *Time) Tick() uint64 {
	return t.tick
}

func (t *Time) Alpha() float64 {
	return t.alpha
}

func (t *Time) Scale() float64 {
	return t.scale
}

func (t *Time) Paused() bool {
	return t.paused
}

func (t *Time) DroppedSteps() uint64 {
	return t.dropped
}

func (t *Time) ClampedTime() float64 {
	return t.clamped
}

func (t *Time) SetScale(s float64) {
	if math.IsNaN(s) || s < 0 {
		s = 0
	}
	t.scale = min(s, MaxScale)
}

func (t *Time) SetPaused(on bool) {
	t.paused = on
	t.pending = stepNone
}

func (t *Time) StepFixed() {
	if t.paused {
		t.pending = stepFixed
	}
}

func (t *Time) StepFrame() {
	if t.paused {
		t.pending = stepFrame
	}
}

func (t *Time) BeginTick() uint64 {
	t.tick++
	return t.tick
}

func (t *Time) drain(limit int) int {
	fd := t.cfg.FixedDelta
	steps := int((t.acc + stepEpsilon) / fd)
	if steps > limit {
		t.dropped += uint64(steps - limit)
		steps = limit
	}
	t.acc -= float64(steps) * fd
	if t.acc < 0 {
		t.acc = 0
	}
	if t.acc >= fd {
		t.acc = math.Mod(t.acc, fd)
	}
	return steps
}

func (t *Time) set(d Domain, delta float64) {
	s := &t.spans[d]
	s.Delta, s.Scale, s.Paused = delta, 1, false
	s.Elapsed += delta
}

func (t *Time) Begin(real float64) Plan {
	if real < 0 || math.IsNaN(real) || math.IsInf(real, 0) {
		real = 0
	}
	if real > t.cfg.MaxFrameDelta {
		t.clamped += real - t.cfg.MaxFrameDelta
		real = t.cfg.MaxFrameDelta
	}
	t.frame++
	fd := t.cfg.FixedDelta
	var plan Plan
	var game float64
	switch {
	case !t.paused:
		game = real * t.scale
		t.acc += game
		plan.Update = true
		plan.Steps = t.drain(t.cfg.MaxFixedSteps)
		if game > 0 {
			t.lastGame = game
		}
	case t.pending == stepFixed:
		plan.Steps = 1
	case t.pending == stepFrame:
		game = t.lastGame
		if game <= 0 {
			game = fd
		}
		t.acc += game
		plan.Update = true
		if t.acc+stepEpsilon >= fd {
			plan.Steps = 1
			t.acc = max(t.acc-fd, 0)
		}
		if t.acc >= fd {
			t.acc = math.Mod(t.acc, fd)
		}
	}
	t.pending = stepNone
	plan.Paused = t.paused && !plan.Update
	fixed := float64(plan.Steps) * fd
	t.set(Real, real)
	t.set(Editor, real)
	t.set(Audio, real)
	g := &t.spans[Game]
	g.Delta, g.Scale, g.Paused = game, t.scale, t.paused
	if plan.Update {
		g.Elapsed += game
	} else {
		g.Elapsed += fixed
	}
	f := &t.spans[Fixed]
	f.Delta, f.Scale, f.Paused = fd, t.scale, t.paused
	f.Elapsed += fixed
	t.alpha = t.acc / fd
	return plan
}
