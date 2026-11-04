package loader

import (
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render/texture"
	"aego/vfs"
)

func newFS(t *testing.T, files map[string][]byte) vfs.FS {
	t.Helper()
	dir := t.TempDir()
	fs, err := vfs.New(vfs.Mount{Scheme: "cache", Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if err := fs.WriteFile("cache://"+name, data); err != nil {
			t.Fatal(err)
		}
	}
	return fs
}

func textureBytes(w, h int, c mathx.Color) []byte {
	d := texture.Solid(w, h, c)
	d.Premultiply()
	return asset.EncodeTexture(&asset.TextureArtifact{
		Width:  w,
		Height: h,
		Format: gpu.TexSRGBA8,
		Pixels: d.Pixels,
	})
}

func newLoader(t *testing.T) (*Loader, *null.Device) {
	t.Helper()
	dev := null.New()
	up := texture.NewUploader(dev)
	l := New(dev, up, log.Nop())
	t.Cleanup(l.Close)
	return l, dev
}

func TestTextureLoadsLazily(t *testing.T) {
	id := ids.NewAssetID()
	fs := newFS(t, map[string][]byte{
		"tex.aetex": textureBytes(8, 8, mathx.Color{R: 1, A: 1}),
	})
	idx := asset.NewIndex()
	idx.Put(asset.Entry{
		ID:   id,
		Kind: asset.ArtTexture,
		Name: "tex",
		Artifacts: []asset.Artifact{
			{Kind: asset.ArtTexture, Path: "cache://tex.aetex"},
		},
	})
	l, _ := newLoader(t)
	if err := l.Mount(idx, fs); err != nil {
		t.Fatal(err)
	}
	if l.Resident() != 0 {
		t.Fatal("loader must not preload anything on mount")
	}
	reg, ok := l.Texture(id)
	if !ok || reg.W != 8 || reg.H != 8 {
		t.Fatalf("region = %+v ok=%v", reg, ok)
	}
	if l.Resident() != 1 {
		t.Fatalf("resident = %d", l.Resident())
	}
	if _, ok := l.Texture(ids.NewAssetID()); ok {
		t.Fatal("unknown asset must not resolve")
	}
}

func TestSpriteRegionResolvesThroughPage(t *testing.T) {
	page := ids.NewAssetID()
	sprite := ids.NewAssetID()
	fs := newFS(t, map[string][]byte{
		"page.aetex": textureBytes(64, 64, mathx.Color{G: 1, A: 1}),
		"spr.aereg": asset.EncodeRegion(&asset.RegionArtifact{
			Page: page, X: 16, Y: 8, W: 32, H: 16, PageW: 64, PageH: 64,
		}),
	})
	idx := asset.NewIndex()
	idx.Put(asset.Entry{
		ID: page, Kind: asset.ArtAtlasPage, Name: "page",
		Artifacts: []asset.Artifact{{Kind: asset.ArtAtlasPage, Path: "cache://page.aetex"}},
	})
	idx.Put(asset.Entry{
		ID: sprite, Kind: asset.ArtSpriteRegion, Name: "spr",
		Artifacts: []asset.Artifact{{Kind: asset.ArtSpriteRegion, Path: "cache://spr.aereg"}},
	})
	l, _ := newLoader(t)
	l.Mount(idx, fs)

	reg, ok := l.Texture(sprite)
	if !ok {
		t.Fatal("sprite region did not resolve")
	}
	if reg.W != 32 || reg.H != 16 {
		t.Fatalf("region size = %dx%d", reg.W, reg.H)
	}
	if mathx.Abs(reg.UVMin.X-0.25) > 1e-6 || mathx.Abs(reg.UVMin.Y-0.125) > 1e-6 {
		t.Fatalf("uv min = %+v", reg.UVMin)
	}
	if mathx.Abs(reg.UVMax.X-0.75) > 1e-6 {
		t.Fatalf("uv max = %+v", reg.UVMax)
	}
}

func TestSwapIsAtomic(t *testing.T) {
	a := ids.NewAssetID()
	b := ids.NewAssetID()
	fs := newFS(t, map[string][]byte{
		"a.aetex": textureBytes(4, 4, mathx.Color{R: 1, A: 1}),
		"b.aetex": textureBytes(4, 4, mathx.Color{B: 1, A: 1}),
	})
	idx := asset.NewIndex()
	for _, e := range []asset.Entry{
		{ID: a, Kind: asset.ArtTexture, Name: "a", Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://a.aetex"}}},
		{ID: b, Kind: asset.ArtTexture, Name: "b", Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://b.aetex"}}},
	} {
		idx.Put(e)
	}
	l, _ := newLoader(t)
	l.Mount(idx, fs)
	before, _ := l.Texture(a)
	genBefore := l.Generation()

	err := l.Swap([]asset.Entry{
		{ID: a, Kind: asset.ArtTexture, Name: "a", Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://a.aetex"}}},
		{ID: b, Kind: asset.ArtTexture, Name: "b", Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://missing.aetex"}}},
	}, nil)
	if err == nil {
		t.Fatal("swap with a missing artifact must fail")
	}
	after, ok := l.Texture(a)
	if !ok || after != before {
		t.Fatal("failed swap must leave the old resource in place")
	}
	if l.Generation() != genBefore {
		t.Fatal("failed swap must not bump the generation")
	}
}

func TestSwapNotifiesOnce(t *testing.T) {
	id := ids.NewAssetID()
	fs := newFS(t, map[string][]byte{
		"a.aetex": textureBytes(4, 4, mathx.Color{R: 1, A: 1}),
	})
	idx := asset.NewIndex()
	entry := asset.Entry{
		ID: id, Kind: asset.ArtTexture, Name: "a",
		Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://a.aetex"}},
	}
	idx.Put(entry)
	l, _ := newLoader(t)
	l.Mount(idx, fs)
	l.Texture(id)

	count := 0
	stop := l.Subscribe(func(r asset.Reloaded) {
		if r.ID != id {
			t.Fatal("wrong asset reported")
		}
		count++
	})
	gen := l.Generation()
	if err := l.Swap([]asset.Entry{entry}, nil); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("notified %d times", count)
	}
	if l.Generation() == gen {
		t.Fatal("successful swap must bump the generation")
	}
	stop()
	l.Swap([]asset.Entry{entry}, nil)
	if count != 1 {
		t.Fatal("unsubscribe failed")
	}
}

func TestRefCachesUntilGenerationChanges(t *testing.T) {
	id := ids.NewAssetID()
	fs := newFS(t, map[string][]byte{
		"a.aetex": textureBytes(4, 4, mathx.Color{R: 1, A: 1}),
	})
	idx := asset.NewIndex()
	entry := asset.Entry{
		ID: id, Kind: asset.ArtTexture, Name: "a",
		Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://a.aetex"}},
	}
	idx.Put(entry)
	l, _ := newLoader(t)
	l.Mount(idx, fs)

	ref := asset.NewRef[asset.Texture](id)
	first, ok := ref.Region(l)
	if !ok {
		t.Fatal("ref did not resolve")
	}
	second, _ := ref.Region(l)
	if first != second {
		t.Fatal("cached region changed without a swap")
	}
	if err := l.Swap([]asset.Entry{entry}, nil); err != nil {
		t.Fatal(err)
	}
	third, ok := ref.Region(l)
	if !ok {
		t.Fatal("ref lost the asset after a swap")
	}
	if third.Texture == first.Texture {
		t.Fatal("swap must give the ref a new texture handle")
	}
}

func TestBrokenArtifactFailsOnceAndIsQuiet(t *testing.T) {
	id := ids.NewAssetID()
	fs := newFS(t, map[string][]byte{"bad.aetex": []byte("not a texture")})
	idx := asset.NewIndex()
	idx.Put(asset.Entry{
		ID: id, Kind: asset.ArtTexture, Name: "bad",
		Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://bad.aetex"}},
	})
	sink := &log.MemorySink{}
	lg := log.New(sink)
	dev := null.New()
	l := New(dev, texture.NewUploader(dev), lg)
	defer l.Close()
	l.Mount(idx, fs)

	for i := 0; i < 5; i++ {
		if _, ok := l.Texture(id); ok {
			t.Fatal("a broken artifact must not resolve")
		}
	}
	if n := len(sink.Records()); n != 1 {
		t.Fatalf("a broken asset must be reported once, got %d records", n)
	}
}

func TestSceneBlob(t *testing.T) {
	id := ids.NewAssetID()
	fs := newFS(t, map[string][]byte{"main.aescene": []byte("scene version=1 {}\n")})
	idx := asset.NewIndex()
	idx.Put(asset.Entry{
		ID: id, Kind: asset.ArtScene, Name: "main",
		Artifacts: []asset.Artifact{{Kind: asset.ArtScene, Path: "cache://main.aescene"}},
	})
	l, _ := newLoader(t)
	l.Mount(idx, fs)

	data, ok := l.Scene(id)
	if !ok || len(data) == 0 {
		t.Fatal("scene blob did not load")
	}
	if _, ok := l.Prefab(id); ok {
		t.Fatal("a scene must not resolve as a prefab")
	}
}

func TestMountRequiresBoth(t *testing.T) {
	l, _ := newLoader(t)
	if err := l.Mount(nil, nil); err == nil {
		t.Fatal("mount without an index must fail")
	}
}

func TestSwapSkipsUnchangedBlobs(t *testing.T) {
	id := ids.NewAssetID()
	fs := newFS(t, map[string][]byte{"main.aescene": []byte("scene version=1 {}\n")})
	idx := asset.NewIndex()
	entry := asset.Entry{
		ID: id, Kind: asset.ArtScene, Name: "main",
		Artifacts: []asset.Artifact{{Kind: asset.ArtScene, Path: "cache://main.aescene"}},
	}
	idx.Put(entry)
	l, _ := newLoader(t)
	l.Mount(idx, fs)
	l.Scene(id)

	count := 0
	l.Subscribe(func(asset.Reloaded) { count++ })
	if err := l.Swap([]asset.Entry{entry}, nil); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("an unchanged scene artifact must not be reported as reloaded")
	}

	fs.WriteFile("cache://main.aescene", []byte("scene version=1 {\n  node \"x\" {}\n}\n"))
	if err := l.Swap([]asset.Entry{entry}, nil); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("a changed scene must be reported once, got %d", count)
	}
}

func TestSwapUncachedSceneUsesPreviousArtifactIdentity(t *testing.T) {
	const body = "scene version=1 {}\n"
	const size = int64(len(body))
	for _, tc := range []struct {
		name             string
		oldKind          asset.ArtifactKind
		oldHash, newHash string
		oldSize          int64
		wantNotified     bool
	}{
		{"identical", asset.ArtScene, "same", "same", size, false},
		{"changed hash", asset.ArtScene, "old", "new", size, true},
		{"changed size", asset.ArtScene, "same", "same", size - 1, true},
		{"changed kind", asset.ArtPrefab, "same", "same", size, true},
		{"missing hashes", asset.ArtScene, "", "", size, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, dep := ids.NewAssetID(), ids.NewAssetID()
			fs := newFS(t, map[string][]byte{"main.aescene": []byte(body)})
			const path = "cache://main.aescene"
			idx := asset.NewIndex()
			idx.Put(asset.Entry{ID: id, Kind: tc.oldKind, Name: "previous", Artifacts: []asset.Artifact{{Kind: tc.oldKind, Path: path, Hash: tc.oldHash, Size: tc.oldSize}}})
			l, _ := newLoader(t)
			if err := l.Mount(idx, fs); err != nil {
				t.Fatal(err)
			}
			if _, loaded := l.blobs[id]; loaded {
				t.Fatal("scene unexpectedly loaded before Swap")
			}
			gen := l.Generation()
			notifications := 0
			stop := l.Subscribe(func(ev asset.Reloaded) {
				if ev.ID != id || ev.Kind != asset.ArtScene {
					t.Fatalf("unexpected reload: %+v", ev)
				}
				notifications++
			})
			defer stop()
			next := asset.Entry{ID: id, Kind: asset.ArtScene, Name: "current", Deps: []ids.AssetID{dep}, Artifacts: []asset.Artifact{{Kind: asset.ArtScene, Path: path, Hash: tc.newHash, Size: size}}}
			if err := l.Swap([]asset.Entry{next}, nil); err != nil {
				t.Fatal(err)
			}
			want := 0
			if tc.wantNotified {
				want = 1
			}
			if notifications != want || l.Generation() != gen+uint32(want) {
				t.Fatalf("reloads=%d generation=%d; want %d reloads from %d", notifications, l.Generation(), want, gen)
			}
			e, found := l.Index().Lookup(id)
			if !found || e.Name != "current" || len(e.Deps) != 1 || e.Deps[0] != dep {
				t.Fatal("no-op content comparison discarded the new index entry")
			}
			if got, ok := l.Scene(id); !ok || string(got) != body {
				t.Fatal("swapped scene bytes do not resolve")
			}
		})
	}
}

var _ = gpu.TexSRGBA8
