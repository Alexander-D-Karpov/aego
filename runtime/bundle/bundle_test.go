package bundle

import (
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/format/aetext"
)

func TestDataRoundTrip(t *testing.T) {
	idx := asset.NewIndex()
	id := ids.NewAssetID()
	idx.Put(asset.Entry{ID: id, Kind: asset.ArtScene, Name: "scenes/main", Source: "scenes/main.aescene",
		Artifacts: []asset.Artifact{{Kind: asset.ArtScene, Path: ArtifactPath("abc.scene"), Hash: "00", Size: 3}}})
	physics := &aetext.Node{Head: "physics", Fields: []aetext.Field{{Name: "gravity", Value: aetext.List(aetext.Int(0), aetext.Int(9))}}}
	in := &Data{Engine: "x", Profile: Release, Target: "linux/amd64", Index: idx, Game: Game{
		Name: "G", Module: "g", Version: "1.2.3", MainScene: id, Window: Window{Width: 640, Height: 360, Title: "T"},
		Defaults: &aetext.Node{Head: "defaults"}, Blocks: []*aetext.Node{physics},
	}}
	raw, err := EncodeData(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeData(raw, ManifestFile)
	if err != nil {
		t.Fatal(err)
	}
	if out.Game.MainScene != id || out.Game.Window.Width != 640 || out.Game.Version != "1.2.3" || !out.Release() {
		t.Fatalf("game block did not survive: %+v", out.Game)
	}
	if out.Game.Block("physics") == nil || out.Game.Block("window") != nil {
		t.Fatal("section blocks are wrong")
	}
	e, ok := out.Index.Lookup(id)
	if !ok || e.Artifacts[0].Path != ArtifactPath("abc.scene") {
		t.Fatalf("index entry lost: %+v", e)
	}
}

func TestExportRoundTrip(t *testing.T) {
	in := &Export{Engine: "e", Game: "G", Target: "darwin/arm64", Profile: Development, Binary: "G.app/Contents/MacOS/g", BinaryGzip: 7,
		Modules: []string{"engine", "script"}, Libs: []Lib{{Name: "SDL3", Link: "static", License: "licenses/SDL3.txt"}},
		Data: []Size{{Kind: "texture", Count: 2, Bytes: 10}}, Artifacts: 2, Hash: "ff"}
	out, err := DecodeExport(EncodeExport(in), ExportFile)
	if err != nil {
		t.Fatal(err)
	}
	if out.Binary != in.Binary || out.BinaryGzip != 7 || len(out.Libs) != 1 || out.Libs[0].License != in.Libs[0].License || out.Data[0] != in.Data[0] || len(out.Modules) != 2 {
		t.Fatalf("export manifest did not survive: %+v", out)
	}
}
