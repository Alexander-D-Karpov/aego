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
		{"physics/components", "physics.CircleCollider2D", "a572890607943e3af0727666c0501e605543dc28a0a9398777ea2fe987e4e213", ""},
		{"gameplay/hitbox", "gameplay.Hitbox2D", "06cebaff0c35ea72d0a8a9112bbbe59eacca4016841c92e25bbe7b6fc8354f35", "10b0b317c79134c3d333f17a42381db8e314e343058a8dcfe7d2806f88da5b1c"},
		{"gameplay/hurtbox", "gameplay.Hurtbox2D", "4cb707ab73abcf3f50af599dc7a6313837044230c95f48d83809ff33d8d3d2a4", "e7a7ad3733ab9f989f50b6f20fed6ed6b9a3cff635c9d04808f336452e287b9e"},
		{"gameplay/killzone", "gameplay.KillZone2D", "ea7ce3acf87237e86e2988a93951ccdfb762b7fe6622ab1a6e84ad3f7ba10fb9", "fe5d7ad926cfc8029e9451992285748c197ef5a018b4eba023a30a74b93d0ecf"},
		{"gameplay/pickup", "gameplay.Pickup2D", "685494b9be74b228fc2762e4cfc870cda000f83afe05c814ddfd190e4ecbbaab", "7b038ac87d73a29ee2044b8ffc66f44b681538f5457631fea471df8808ddd33a"},
		{"gameplay/checkpoint", "gameplay.Checkpoint2D", "9d35c26516469c4180cfbdb94edf524d8b54f47f2c897a946e93b829152a974f", "951d0c477e0baeeced3c7a64093e324d30588844af32f952279985cf9a62fa61"},
		{"gameplay/respawn", "gameplay.Respawn", "c604b98824cc3483b506e1f529e51159cbe8d3c8e058a37eb437c0e35890089a", "d13601abe19642eee5a4f3cd7056e0812dc93f5f59f6e904c972d1e7b09cc260"},
		{"audio", "audio.AudioSource2D", "0c43758ee5bf1ad7304668d36b138a3f417dc795e56730d2226ca935e204cf7e", "1f0bde6059abe3241de2e799bd9e3e06f5a4259176bb636ba3e0d60fa7d0750c"},
		{"audio/spatial", "audio.AudioListener2D", "3ffe126eea8b20663538fd4e1b2d083c9157edd2f7adfbe983bfec962229ff69", "2e79adf7b30530df3479608124ec67678cf9ef7847b570866731cc402652cac3"},
		{"audio/zones", "audio.AudioZone2D", "2181dfaf04ca60b26b93a263c58d686423babf61139b9662b8502cd67ba2228c", "994b455b9af397862a58d284f763fd26b05aa4d51e80965464c4ff9379f80c58"},
		{"save", "save.Persist", "19499400ad68dded38129ccec04546a5b018add92114f2e21f0c8bf6f61506cb", "8cde05fe8d4c3f6e6b722ff60b8be7a687160e50fefd5a09001f25a76299675a"},
		{"tags/example", "engine.Tags", "e642c5c9983312c9f75b7966b3a9ecfeb2f08196f2cb7ebe9e3bf0adc24bdbbe", "a6459199adcbbd52aab200b5c04e331b478b4e1b4b0ddb6a688e7028416a5215"},
		{"language/script-example", "engine.Script", "8927658496fa7c6afc6a77ba8818018faa5b93936c8a32a13bd72f5ec7f59d6d", "18f02ddc587e8719b73b9d68d749f533be2290512075403a1560191c05655ca8"},
		{"render2d/materials", "engine.MaterialOverride", "c0a63c2b8cfdf22028ba1df4c7152a75c1f69a973c70175ec44f476aadf19405", "b5c1b4249a9b3232b9845f2a61b2cf2a5af48c3d9d065bf95f7ff877305de3b3"},
		{"prefabs/example", "engine.PrefabInstance", "90289773f897db2a386ad6cf9380dbc874f03f6f0cb76a03d2c2d08a1de833da", "5e258045feacf7591993ce7397df8aed9e5247d776fe2d2055b78b55571d1ea1"},
	} {
		t.Run(spec.guide, func(t *testing.T) {
			guide, ok := p.Docs().Get(spec.guide)
			if !ok {
				t.Fatal("guide not found")
			}
			wantSelect := spec.component
			if spec.guide == "render2d/materials" {
				wantSelect = "" // This authored guide never specified a component selection.
			}
			if got := fmt.Sprintf("%x", sha256.Sum256([]byte(guide.Try))); got != spec.sum || guide.TrySelect != wantSelect {
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
