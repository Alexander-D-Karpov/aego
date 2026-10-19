package preview

import (
	"aego/core/mathx"
	"aego/render/rt"
	"aego/render/view"
	"bytes"
	"testing"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
)

func assetInfo(k asset.ArtifactKind) pipeline.AssetInfo {
	return pipeline.AssetInfo{ID: ids.NewAssetID(), Kind: k}
}

func TestKeyFollowsDeps(t *testing.T) {
	s := &Service{items: map[ids.AssetID]*item{}}
	e := asset.Entry{ID: ids.NewAssetID(), Kind: asset.ArtTexture, Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Hash: "a"}}}
	k1, ok := s.Key(e)
	if !ok {
		t.Fatal("texture must have a key")
	}
	e.Artifacts[0].Hash = "b"
	if k2, _ := s.Key(e); k2 == k1 {
		t.Fatal("key must change with the artifact hash")
	}
	e.Artifacts = nil
	if _, ok := s.Key(e); ok {
		t.Fatal("an entry without its artifact has no key")
	}
}

func TestCacheRoundTrip(t *testing.T) {
	s := &Service{dir: t.TempDir()}
	src := pixels{buf: bytes.Repeat([]byte{10, 20, 30, 255}, 4*3), w: 4, h: 3}
	if err := s.save("k", src); err != nil {
		t.Fatal(err)
	}
	got, err := s.load("k")
	if err != nil || got.w != 4 || got.h != 3 || !bytes.Equal(got.buf, src.buf) {
		t.Fatalf("cache round trip failed: %+v %v", got, err)
	}
}

func TestSchematicWithoutGPU(t *testing.T) {
	s := New(Deps{})
	for _, k := range []asset.ArtifactKind{asset.ArtScene, asset.ArtPrefab, asset.ArtData, asset.ArtTable} {
		if !s.Missing(assetInfo(k)) {
			t.Fatalf("%s must fall back to the schematic preview without a GPU", k)
		}
	}
	if s.Missing(assetInfo(asset.ArtTexture)) {
		t.Fatal("textures are decoded on the CPU and need no GPU")
	}
}

func TestFirstLines(t *testing.T) {
	got := firstLines("package hello\n\n// doc\nscript Spinner {\n\tvar Speed float = 90\n\n\tfunc Update(dt float) {\n", 3)
	if len(got) != 3 || got[0] != "script Spinner {" || got[1] != "  var Speed float = 90" {
		t.Fatalf("lines = %q", got)
	}
}

func TestCameraViewFitsWindowHeight(t *testing.T) {
	tgt := &rt.Target{}
	in := view.View{Camera: view.Camera2D{Position: mathx.Vec2{X: 3, Y: 4}, Zoom: 64}}
	v, ok := cameraView([]view.View{in}, mathx.Vec2{X: 1280, Y: 720}, tgt)
	if !ok {
		t.Fatal("a 2D scene camera must give the preview view")
	}
	if want := float32(64) * Size / 720; v.Camera.Zoom != want || v.Camera.Position != in.Camera.Position {
		t.Fatalf("zoom = %v, want %v, position kept", v.Camera.Zoom, want)
	}
	if v.Viewport.W != Size || v.Viewport.H != Size {
		t.Fatal("the view must cover the preview target")
	}
}

func TestFirstLinesSkipImports(t *testing.T) {
	got := firstLines("import (\n\t\"aego/math\"\n)\nimport \"aego/time\"\n\nscript Laser {\n", 2)
	if len(got) != 1 || got[0] != "script Laser {" {
		t.Fatalf("lines = %q", got)
	}
}
