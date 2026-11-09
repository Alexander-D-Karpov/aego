package remoteexample

import "testing"

// The producer and the receiving validator must agree on the same path policy.
// Otherwise an example could be published successfully but rejected by clients,
// or a publisher could bypass the restrictions enforced on downloaded files.
func TestResourcePolicyMatchesProducerAndManifest(t *testing.T) {
	const origin = "https://examples.test"
	const manifestURL = origin + "/manifest.json"
	for _, tc := range []struct {
		name string
		ok   bool
	}{
		{"assets/player.png", true},
		{"assets/animation.ASEPRITE", true},
		{"assets/remote-example.receipt.json", true},
		{"LICENSE", true},
		{"../outside.txt", false},
		{"/absolute.txt", false},
		{"assets/CLOCK$.txt", false},
		{"assets/scene.aescene:ads", false},
		{"assets/рисунок.png", false},
		{".git/config", false},
		{".env.local", false},
		{"native/addon.dll", false},
		{"remote-example.receipt.json", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string][]byte{
				"project.aego":        []byte("project {}"),
				"scenes/main.aescene": []byte("scene {}"),
				tc.name:               []byte("resource"),
			}
			m := exampleManifest(origin, []byte("archive"), files)
			manifestErr := m.Validate(manifestURL, Limits{})
			_, _, _, buildErr := BuildPackage(m, files, manifestURL, Limits{})
			if (manifestErr == nil) != tc.ok || (buildErr == nil) != tc.ok {
				t.Fatalf("accepted=%v, manifest error=%v, builder error=%v", tc.ok, manifestErr, buildErr)
			}
			if manifestErr != nil && manifestErr.Error() != buildErr.Error() {
				t.Fatalf("path rejection differs: manifest=%v, builder=%v", manifestErr, buildErr)
			}
		})
	}
}
