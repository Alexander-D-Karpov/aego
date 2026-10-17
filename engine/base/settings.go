package base

import (
	"aego/core/errs"
	"aego/engine"
	"aego/engine/module"
	"aego/schema"
)

const Section = CoreID

type Settings struct {
	FixedHz       float64 `aego:"fixedHz"`
	MaxFrameDelta float64 `aego:"maxFrameDelta"`
	MaxFixedSteps int32   `aego:"maxFixedSteps"`
	CrashScene    bool    `aego:"crashScene"`
	SpikeMs       float64 `aego:"spikeMs"`
}

func DefaultSettings() Settings {
	return Settings{CrashScene: true}
}

func (s *Settings) Apply(c *engine.Config) {
	if s.FixedHz > 0 {
		c.FixedHz = s.FixedHz
	}
	if s.MaxFrameDelta > 0 {
		c.MaxFrameDelta = s.MaxFrameDelta
	}
	if s.MaxFixedSteps > 0 {
		c.MaxFixedSteps = int(s.MaxFixedSteps)
	}
}

func validate(s *Settings) error {
	switch {
	case !(s.FixedHz >= 0 && s.FixedHz <= 1000):
		return errs.New(errs.ProjectBadFile, "engine: fixedHz must be between 0 and 1000")
	case !(s.MaxFrameDelta >= 0 && s.MaxFrameDelta <= 1):
		return errs.New(errs.ProjectBadFile, "engine: maxFrameDelta must be between 0 and 1 second")
	case s.MaxFixedSteps < 0 || s.MaxFixedSteps > 64:
		return errs.New(errs.ProjectBadFile, "engine: maxFixedSteps must be between 0 and 64")
	case !(s.SpikeMs >= 0):
		return errs.New(errs.ProjectBadFile, "engine: spikeMs must not be negative")
	}
	return nil
}

func registerSettings(r *module.Registry) error {
	if err := module.Struct[Settings](r,
		schema.Name("engine.Settings"),
		schema.FieldDoc("fixedHz", "Fixed steps per second. Zero uses 60."),
		schema.FieldDoc("maxFrameDelta", "Longest frame the clock accepts, in seconds; longer frames are cut. Zero uses 0.1."),
		schema.FieldDoc("maxFixedSteps", "Most fixed steps per frame; extra steps are dropped and counted. Zero uses 4."),
		schema.FieldDoc("crashScene", "Crash bundles include the play scene."),
		schema.FieldDoc("spikeMs", "The editor writes a bundle when a frame takes longer than this, at most once per 10 seconds. Zero turns it off."),
	); err != nil {
		return err
	}
	r.ProjectSection(module.SectionDesc[Settings]{ID: Section, Title: "Engine", Order: 50, Default: DefaultSettings, Validate: validate})
	return nil
}
