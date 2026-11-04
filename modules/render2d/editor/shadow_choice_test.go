package editor

import (
	"strings"
	"testing"

	"aego/render"
	"aego/scene"
	"aego/schema"
)

func TestShadowChoiceKeepsLightAcrossLayerReordering(t *testing.T) {
	s := scene.New(scene.NewTypes(schema.NewRegistry()), nil, scene.Options{})
	left := s.Create(s.Root(), "Lantern")
	right := s.Create(s.Root(), "PlayerSpot")
	s.Flush()
	a, b := s.ID(left), s.ID(right)
	p := &shadowResourcePreview{light: b, layers: []render.ShadowInfo{{Light: a, Layer: 0}, {Light: b, Layer: 1}}}
	p.syncChoices(s)
	if p.index != 1 || !strings.Contains(p.labels[1], "PlayerSpot") {
		t.Fatal(p.index, p.labels)
	}
	p.layers[0].Light, p.layers[1].Light = b, a
	s.SetName(right, "HeroSpot")
	p.syncChoices(s)
	if p.index != 0 || !strings.Contains(p.labels[0], "HeroSpot") || !strings.Contains(p.labels[1], "Lantern") {
		t.Fatal(p.index, p.labels)
	}
}
