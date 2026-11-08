package document

import (
	"aego/asset"
	"testing"
)

func TestSourceEditorDoesNotReplacePreparedImageViewer(t *testing.T) {
	var r HandlerRegistry
	if err := r.Register(Handler{ID: "image", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Extensions: []string{".png"}, Capabilities: View}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(Handler{ID: "art", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Extensions: []string{".aeart"}, SourceExtensionsOnly: true, Capabilities: View | EditSource}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ file, want string }{{"assets/hero.png", "image"}, {"assets/hero.aeart", "art"}, {"assets/HERO.AEART", "art"}} {
		if got := r.Resolve(asset.ArtTexture, tc.file, "").ID; got != tc.want {
			t.Fatalf("%s: %s, want %s", tc.file, got, tc.want)
		}
	}
	if err := r.Register(Handler{ID: "ambiguous-art", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Extensions: []string{".AEART"}, SourceExtensionsOnly: true, Capabilities: View}); err == nil {
		t.Fatal("duplicate source editor accepted")
	}
	if err := r.Register(Handler{ID: "other-source", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Extensions: []string{".different"}, SourceExtensionsOnly: true, Capabilities: View}); err != nil {
		t.Fatal(err)
	}
}
