package debug

import (
	"aego/core/errs"
	"aego/engine/module"
	"aego/schema"
)

type Settings struct {
	Enabled       bool  `aego:"enabled"`
	MaxPrimitives int32 `aego:"maxPrimitives"`
	MaxPerChannel int32 `aego:"maxPerChannel"`
}

func DefaultSettings() Settings {
	return Settings{Enabled: true, MaxPrimitives: 16384, MaxPerChannel: 4096}
}

func (s Settings) normalized() Settings {
	d := DefaultSettings()
	if s.MaxPrimitives <= 0 {
		s.MaxPrimitives = d.MaxPrimitives
	}
	if s.MaxPerChannel <= 0 {
		s.MaxPerChannel = d.MaxPerChannel
	}
	return s
}

func validate(s *Settings) error {
	if s.MaxPrimitives < 0 || s.MaxPrimitives > 1<<20 || s.MaxPerChannel < 0 {
		return errs.New(errs.ProjectBadFile, "debug: maxPrimitives must be between 0 and 1048576, maxPerChannel must not be negative")
	}
	return nil
}

func registerSettings(r *module.Registry) error {
	if err := module.Struct[Settings](r,
		schema.Name("debug.Settings"),
		schema.FieldDoc("enabled", "Turns debug drawing on. When off, every draw call does nothing."),
		schema.FieldDoc("maxPrimitives", "Primitives kept per world. Extra primitives are dropped and counted. Zero uses 16384."),
		schema.FieldDoc("maxPerChannel", "Primitives kept per channel. Zero uses 4096."),
	); err != nil {
		return err
	}
	r.ProjectSection(module.SectionDesc[Settings]{ID: ID, Title: "Debug Draw", Order: 400, Default: DefaultSettings, Validate: validate})
	return nil
}
