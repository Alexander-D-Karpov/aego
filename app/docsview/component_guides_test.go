//go:build aego_editor

package docsview

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"aego/app/playground"
)

// Component-specific demonstrations must not silently replace the existing
// authored scenes and scripts used by the broader guides.
func TestComponentTryGuidesPreserveAuthoredScenes(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range []struct{ guide, component, sum, script string }{
		{"camera", "camera.CameraFollow2D", "0a7ed8d6fd9cce7ee144cd2a0ca91a6ff377220cc7b737ebe9feb93e10331f3a", ""},
		{"camera/follow", "camera.CameraFollow2D", "0a7ed8d6fd9cce7ee144cd2a0ca91a6ff377220cc7b737ebe9feb93e10331f3a", ""},
		{"camera/zones", "camera.CameraZone2D", "9f5b36a8de5584836d466b8437510c1990e3c719d41a4c65dfa591b2b7c634e8", ""},
		{"camera/outputs", "camera.CameraBrain2D", "d475b935c8c41973653b44aa1ff75d4871263a0ad2cb9d56097f234818be1aa4", ""},
		{"camera/virtual", "camera.VirtualCamera2D", "14e775168e8f7f5250d42970db4578d39219a9e3bb9d0dab839892fd92ae88a8", "af3febb779542a8605bd2899667e87d5cf81826ca4d15efcbab85a0c0efcffab"},
		{"physics", "physics.Body2D", "87081fb3f66fb919ef02833f30d052457b456c841b2b6406c9930ad47884644c", ""},
	} {
		t.Run(spec.guide, func(t *testing.T) {
			guide, ok := p.Docs().Get(spec.guide)
			if !ok {
				t.Fatal("guide not found")
			}
			if got := fmt.Sprintf("%x", sha256.Sum256([]byte(guide.Try))); got != spec.sum || guide.TrySelect != spec.component {
				t.Fatalf("authored guide scene or selection changed: sha=%s select=%s", got, guide.TrySelect)
			}
			if spec.script != "" {
				if got := fmt.Sprintf("%x", sha256.Sum256([]byte(guide.TryScript))); got != spec.script {
					t.Fatalf("authored guide script changed: sha=%s", got)
				}
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
