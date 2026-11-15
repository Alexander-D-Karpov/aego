package art

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"slices"
)

// EffectTemplateProperty is preserved by the ordinary document serializer. It
// selects a built-in particle preset, not executable code or an external path.
const EffectTemplateProperty = "aego.effectTemplate"

type EffectTemplate struct {
	ID             string `json:"id"`
	NameEN         string `json:"nameEN"`
	NameRU         string `json:"nameRU"`
	ParticlePreset string `json:"particlePreset"`
}

var effectTemplates = []EffectTemplate{
	{"dust", "Dust", "Пыль", "Dust"},
	{"spark", "Spark", "Искры", "Spark"},
	{"smoke", "Smoke", "Дым", "Smoke"},
	{"explosion", "Explosion", "Взрыв", "Explosion"},
	{"fire", "Fire", "Огонь", "Fire"},
	{"hit-flash", "Hit Flash", "Вспышка удара", "HitFlash"},
	{"magic-projectile", "Magic Projectile", "Магический снаряд", "MagicProjectile"},
	{"trail", "Trail", "След", "Trail"},
}

//go:embed res/effects
var effectSources embed.FS

// EffectTemplates returns detached, localized catalogue entries shared by all
// hosts. The actual editable artwork is embedded once beside the Art Core.
func EffectTemplates() []EffectTemplate { return slices.Clone(effectTemplates) }

func EffectTemplateOf(id string) (EffectTemplate, bool) {
	for _, template := range effectTemplates {
		if template.ID == id {
			return template, true
		}
	}
	return EffectTemplate{}, false
}

// EffectTemplateBytes returns an independent copy of the versioned starter
// document. Release builders can distribute these exact, reproducible bytes.
func EffectTemplateBytes(id string) ([]byte, error) {
	if _, ok := EffectTemplateOf(id); !ok {
		return nil, fmt.Errorf("art: unknown effect template %q", id)
	}
	return effectSources.ReadFile("res/effects/" + id + SourceExtension)
}

// NewEffectDocument makes a fully independent drawing with a new document
// identity. Cels, palettes and pixels use the normal decoder and COW model;
// editing one instance never modifies another instance or the starter bytes.
func NewEffectDocument(id string, limits Limits) (*Document, error) {
	data, err := EffectTemplateBytes(id)
	if err != nil {
		return nil, err
	}
	doc, err := Decode(data, limits)
	if err != nil {
		return nil, err
	}
	var identity [16]byte
	if _, err = rand.Read(identity[:]); err != nil {
		doc.Close()
		return nil, err
	}
	doc.id = hex.EncodeToString(identity[:])
	return doc, nil
}
