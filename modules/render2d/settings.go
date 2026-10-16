package render2d

import (
	"aego/core/errs"
	"aego/engine/module"
	"aego/schema"
)

const (
	SectionID            = ID
	DefaultPixelsPerUnit = 64
)

type Settings struct {
	PixelsPerUnit int32 `aego:"pixelsPerUnit,min=0"`
}

func DefaultSettings() Settings {
	return Settings{PixelsPerUnit: DefaultPixelsPerUnit}
}

func (s Settings) PPU() float32 {
	if s.PixelsPerUnit <= 0 {
		return DefaultPixelsPerUnit
	}
	return float32(s.PixelsPerUnit)
}

func validateSettings(s *Settings) error {
	if s.PixelsPerUnit < 0 {
		return errs.New(errs.ProjectBadFile, "render2d: pixelsPerUnit must not be negative")
	}
	return nil
}

func registerSettings(r *module.Registry) error {
	if err := module.Struct[Settings](r,
		schema.Name("render2d.Settings"),
		schema.FieldDoc("pixelsPerUnit", "Texture pixels per world unit. A texture dropped into the scene gets size = pixels / this value. Zero uses 64."),
	); err != nil {
		return err
	}
	r.ProjectSection(module.SectionDesc[Settings]{
		ID:       SectionID,
		Former:   []string{"render"},
		Title:    "Render",
		Order:    100,
		Default:  DefaultSettings,
		Validate: validateSettings,
	})
	return nil
}
