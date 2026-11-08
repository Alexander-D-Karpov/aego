package agent

import (
	"fmt"
	"testing"

	"aego/art"
	"aego/asset"
)

// Source-only scene validation must not treat a partial sprite sheet as a
// complete generic material texture when native runtime rejects that sampler.
func TestAgentSourceMaterialRejectsPartialArtSheet(t *testing.T) {
	for _, multi := range []bool{false, true} {
		t.Run(fmt.Sprintf("multi=%v", multi), func(t *testing.T) {
			s := materialSession(t)
			doc, err := art.New(4, 4, art.RGBA, art.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			defer doc.Close()
			if multi {
				edit, err := doc.Begin(doc.Revision(), "Add frame")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := edit.AddFrame(doc.Info().Frames[0].ID, 100); err != nil {
					t.Fatal(err)
				}
				change, err := edit.Prepare()
				if err != nil {
					t.Fatal(err)
				}
				if err := change.Apply(); err != nil {
					t.Fatal(err)
				}
			}
			data, err := doc.Encode()
			if err != nil {
				t.Fatal(err)
			}
			artID, err := s.create("assets/art.aeart", data)
			if err != nil {
				t.Fatal(err)
			}
			content := fmt.Sprintf("material version=1 shader=asset(%q) { params { Noise=asset(%q) } }", asset.BuiltinShaderDissolve.String(), artID.String())
			materialID, err := s.create("assets/art.aemat", []byte(content))
			if err != nil {
				t.Fatal(err)
			}
			if info, ok := (sources{s.Pipeline}).TextureMetadata(artID); !ok || info.Region != multi || info.Width != 4 || info.Height != 4 {
				t.Fatalf("source-only metadata lost first-frame bounds: %+v/%v", info, ok)
			}
			if _, ok := (sources{s.Pipeline}).Material(materialID); ok == multi {
				t.Fatalf("source-only material resolver result=%v for multi=%v", ok, multi)
			}
		})
	}
}
