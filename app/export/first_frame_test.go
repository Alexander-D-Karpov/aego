package export

import (
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/gpu"
	"aego/vfs"
	"aego/vfs/memfs"
)

// Export must validate the same source frame that runtime renders, rather than
// compare its 4x4 normal with the dimensions of the entire 8x4 sheet.
func TestExportSourceFirstFrameNormalAssociation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		normalW    int
		semantic   asset.TextureSemantic
		wantBlocks bool
	}{
		{"matching", 4, asset.TextureNormal, false},
		{"wrong-size", 3, asset.TextureNormal, true},
		{"wrong-semantic", 4, asset.TextureColor, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, normal, normalPage := ids.NewAssetID(), ids.NewAssetID(), ids.NewAssetID()
			texture := func(semantic asset.TextureSemantic) []byte {
				return asset.EncodeTexture(&asset.TextureArtifact{Width: 8, Height: 4, Format: gpu.TexRGBA8, Semantic: semantic, Pixels: make([]byte, 8*4*4)})
			}
			files := map[string][]byte{
				"sheet":        texture(asset.TextureColor),
				"source-frame": asset.EncodeRegion(&asset.RegionArtifact{Page: root, W: 4, H: 4, PageW: 8, PageH: 4, Normal: normal}),
				"normal-sheet": texture(tc.semantic),
				"normal-frame": asset.EncodeRegion(&asset.RegionArtifact{Page: normalPage, W: tc.normalW, H: 4, PageW: 8, PageH: 4}),
			}
			fs, err := vfs.New(vfs.Mount{Scheme: "cache", Source: memfs.New(files)})
			if err != nil {
				t.Fatal(err)
			}
			idx := asset.NewIndex()
			idx.Put(asset.Entry{ID: root, Kind: asset.ArtTexture, Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://sheet"}, {Kind: asset.ArtSpriteRegion, Path: "cache://source-frame"}}})
			idx.Put(asset.Entry{ID: normalPage, Kind: asset.ArtTexture, Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://normal-sheet"}}})
			idx.Put(asset.Entry{ID: normal, Kind: asset.ArtSpriteRegion, Artifacts: []asset.Artifact{{Kind: asset.ArtSpriteRegion, Path: "cache://normal-frame"}}})
			snap := &Snapshot{idx: idx, files: fs}
			if got, ok := (&resolver{s: snap}).textureInfo(root); !ok || !got.region || got.w != 4 || got.h != 4 {
				t.Fatalf("export source metadata differs from its first frame: %+v/%v", got, ok)
			}
			problems := checkNormalAssociations(snap)
			if (Blocking(problems) != 0) != tc.wantBlocks {
				t.Fatalf("normal validation: got %+v; want blocking=%v", problems, tc.wantBlocks)
			}
		})
	}
}
