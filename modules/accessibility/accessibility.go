package accessibility

import (
	"aego/engine"
	"aego/engine/module"
	"aego/engine/settings"
	"aego/schema"
)

const (
	ID               = "accessibility"
	KeyReducedMotion = ID + ".reducedMotion"
	KeyScreenShake   = ID + ".screenShake"
	KeyHighContrast  = ID + ".highContrast"
	KeyColorblind    = ID + ".colorblindPalette"
	KeySubtitles     = ID + ".subtitles"
)

type Module struct{}

func New() Module {
	return Module{}
}

func (Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Builtin: true, DisplayName: "Accessibility", DocsOrder: 140}
}

func (Module) Register(r *module.Registry) error {
	both := settings.User | settings.Project
	cat := settings.CategoryAccessibility
	r.Setting(settings.Desc{ID: KeyReducedMotion, Kind: schema.KindBool, Title: "Reduce Motion", Category: cat, Order: 10, Flags: both,
		Doc: "Cameras, particles and transitions cut large or fast motion."})
	r.Setting(settings.Desc{ID: KeyScreenShake, Kind: schema.KindFloat64, Default: schema.Float64(1), Title: "Screen Shake", Category: cat, Order: 20,
		Flags: both, Min: 0, Max: 1, HasMin: true, HasMax: true, Step: 0.05})
	r.Setting(settings.Desc{ID: KeyHighContrast, Kind: schema.KindBool, Title: "High Contrast", Category: cat, Order: 30, Flags: both})
	r.Setting(settings.Desc{ID: KeyColorblind, Kind: schema.KindEnum, Title: "Colorblind Palette", Category: cat, Order: 40, Flags: both,
		Choices: []settings.Choice{
			{Name: "none", Label: "None", Value: schema.Enum(0)},
			{Name: "protanopia", Label: "Protanopia", Value: schema.Enum(1)},
			{Name: "deuteranopia", Label: "Deuteranopia", Value: schema.Enum(2)},
			{Name: "tritanopia", Label: "Tritanopia", Value: schema.Enum(3)},
		}})
	r.Setting(settings.Desc{ID: KeySubtitles, Kind: schema.KindBool, Title: "Subtitles", Category: cat, Order: 50, Flags: both})
	return nil
}
