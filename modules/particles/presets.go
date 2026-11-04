package particles

import (
	"embed"
	"fmt"

	"aego/asset"
	"aego/core/ids"
	"aego/format/aetext"
	"aego/scene"
	"aego/scene/sceneio"
)

//go:embed res/presets/*.aepreset
var sources embed.FS

var presetNames = [...]string{"Dust", "Spark", "Smoke", "Rain", "Explosion"}

func PresetSource(name string) ([]byte, error) {
	for _, p := range presetNames {
		if p == name {
			return sources.ReadFile("res/presets/" + p + ".aepreset")
		}
	}
	return nil, fmt.Errorf("unknown particle preset %q", name)
}

func ReadPreset(types *scene.Types, resolver asset.Resolver, data []byte) (ParticleSystem2D, error) {
	doc, err := aetext.Parse(data, "effect.aepreset")
	if err != nil {
		return ParticleSystem2D{}, err
	}
	component, err := asset.PresetComponent(doc, NameSystem)
	if err != nil {
		return ParticleSystem2D{}, err
	}
	td, ok := types.Schema().ByName(NameSystem)
	if !ok {
		return ParticleSystem2D{}, fmt.Errorf("component %s is not registered", NameSystem)
	}
	if err = sceneio.ValidatePresetFields(types.Schema(), td, component); err != nil {
		return ParticleSystem2D{}, err
	}
	sc, report, err := sceneio.Load(data, "effect.aepreset", types, resolver, sceneio.LoadOptions{})
	if err != nil {
		return ParticleSystem2D{}, err
	}
	if len(report.Warnings) > 0 || len(report.Unknown) > 0 || len(report.Missing) > 0 {
		return ParticleSystem2D{}, fmt.Errorf("invalid effect preset: %+v", report)
	}
	var out ParticleSystem2D
	count := 0
	scene.Store[ParticleSystem2D](sc).EachAll(func(_ scene.NodeHandle, c *ParticleSystem2D) { out = *c; count++ })
	if count != 1 {
		return ParticleSystem2D{}, fmt.Errorf("expected one %s in preset, found %d", NameSystem, count)
	}
	return out, out.Validate()
}

func LoadPreset(types *scene.Types, res asset.Resolver, id ids.AssetID) (ParticleSystem2D, error) {
	if src, ok := res.(asset.PresetSource); ok {
		if data, ok := src.Preset(id); ok {
			return ReadPreset(types, res, data)
		}
	}
	return ParticleSystem2D{}, fmt.Errorf("particle preset %s is unresolved", id)
}
