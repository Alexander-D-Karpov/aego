package clock

import (
	"math"
	"time"
)

const (
	DefaultFixedDelta    = 1.0 / 60
	DefaultMaxFrameDelta = 0.1
	DefaultMaxSteps      = 4
	stepEpsilon          = 1e-9
)

type Clock struct {
	start time.Time
}

func New() *Clock {
	return &Clock{start: time.Now()}
}

func (c *Clock) Now() float64 {
	return time.Since(c.start).Seconds()
}

type FixedStepper struct {
	FixedDelta    float64
	MaxFrameDelta float64
	MaxSteps      int
	accumulator   float64
}

func (s *FixedStepper) Advance(frameDelta float64) (steps int, alpha float64) {
	s.applyDefaults()
	if frameDelta < 0 || math.IsNaN(frameDelta) {
		frameDelta = 0
	}
	if frameDelta > s.MaxFrameDelta {
		frameDelta = s.MaxFrameDelta
	}
	s.accumulator += frameDelta
	for s.accumulator+stepEpsilon >= s.FixedDelta && steps < s.MaxSteps {
		s.accumulator -= s.FixedDelta
		steps++
	}
	if s.accumulator < 0 {
		s.accumulator = 0
	}
	if s.accumulator >= s.FixedDelta {
		s.accumulator = math.Mod(s.accumulator, s.FixedDelta)
	}
	return steps, s.accumulator / s.FixedDelta
}

func (s *FixedStepper) Reset() {
	s.accumulator = 0
}

func (s *FixedStepper) Accumulator() float64 {
	return s.accumulator
}

func (s *FixedStepper) applyDefaults() {
	if s.FixedDelta <= 0 {
		s.FixedDelta = DefaultFixedDelta
	}
	if s.MaxFrameDelta <= 0 {
		s.MaxFrameDelta = DefaultMaxFrameDelta
	}
	if s.MaxSteps <= 0 {
		s.MaxSteps = DefaultMaxSteps
	}
}
