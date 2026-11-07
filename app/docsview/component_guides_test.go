//go:build aego_editor

package docsview

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"aego/app/playground"
)

// Component-specific demonstrations must not silently replace the existing
// rolling/falling scenes used by the broader guides.
func TestComponentTryGuidesPreserveAuthoredScenes(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range []struct{ guide, component, sum string }{
		{"camera", "camera.CameraFollow2D", "0a7ed8d6fd9cce7ee144cd2a0ca91a6ff377220cc7b737ebe9feb93e10331f3a"},
		{"camera/follow", "camera.CameraFollow2D", "0a7ed8d6fd9cce7ee144cd2a0ca91a6ff377220cc7b737ebe9feb93e10331f3a"},
		{"camera/zones", "camera.CameraZone2D", "9f5b36a8de5584836d466b8437510c1990e3c719d41a4c65dfa591b2b7c634e8"},
		{"physics", "physics.Body2D", "87081fb3f66fb919ef02833f30d052457b456c841b2b6406c9930ad47884644c"},
	} {
		t.Run(spec.guide, func(t *testing.T) {
			guide, ok := p.Docs().Get(spec.guide)
			if !ok {
				t.Fatal("guide not found")
			}
			if got := fmt.Sprintf("%x", sha256.Sum256([]byte(guide.Try))); got != spec.sum || guide.TrySelect != spec.component {
				t.Fatalf("authored guide scene or selection changed: sha=%s select=%s", got, guide.TrySelect)
			}
			component, ok := p.Docs().Get(spec.component)
			if !ok || component.Demo == nil || component.Try == guide.Try {
				t.Fatalf("%s needs its distinct executable component demo", spec.component)
			}
		})
	}
	for _, problem := range p.Docs().Problems() {
		if problem.Error {
			t.Fatalf("documentation registry error: %+v", problem)
		}
	}
}
