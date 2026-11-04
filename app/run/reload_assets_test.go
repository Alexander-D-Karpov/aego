package run

import (
	"testing"

	"aego/app"
	"aego/asset"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render/texture"
	"aego/vfs"
	"aego/vfs/memfs"
)

func TestReloadRemovesOldAtlasOnlyAfterSuccessfulSwap(t *testing.T) {
	old, replacement := ids.NewAssetID(), ids.NewAssetID()
	image := asset.EncodeTexture(&asset.TextureArtifact{Width: 1, Height: 1, Format: gpu.TexRGBA8, Pixels: []byte{255, 255, 255, 255}})
	files, err := vfs.New(vfs.Mount{Scheme: "cache", Source: memfs.New(map[string][]byte{"old.aetex": image, "bad.aetex": []byte("broken")})})
	if err != nil {
		t.Fatal(err)
	}
	index := asset.NewIndex()
	index.Put(asset.Entry{ID: old, Kind: asset.ArtAtlasPage, Artifacts: []asset.Artifact{{Kind: asset.ArtAtlasPage, Path: "cache://old.aetex"}}})
	device := null.New()
	l := loader.New(device, texture.NewUploader(device), log.Nop())
	defer l.Close()
	if err := l.Mount(index, files); err != nil {
		t.Fatal(err)
	}
	before, ok := l.Texture(old)
	if !ok {
		t.Fatal("old atlas not loaded")
	}
	a := &App{Host: &app.Host{Pipeline: &pipeline.Pipeline{}, Loader: l}, log: log.Nop().With(log.Engine)}
	a.applyBatch(&pipeline.Batch{Removed: []ids.AssetID{old}, Imported: []asset.Entry{{ID: replacement, Kind: asset.ArtAtlasPage, Artifacts: []asset.Artifact{{Kind: asset.ArtAtlasPage, Path: "cache://bad.aetex"}}}}})
	if after, ok := l.Texture(old); !ok || after != before {
		t.Fatal("failed candidate deleted the live atlas")
	}
	a.applyBatch(&pipeline.Batch{Removed: []ids.AssetID{old}})
	if _, ok := l.Texture(old); ok {
		t.Fatal("a removal-only batch left the deleted atlas resident")
	}
}
