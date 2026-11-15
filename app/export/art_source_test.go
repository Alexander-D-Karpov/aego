package export

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/animclip"
	"aego/asset/importers/artsource"
	"aego/asset/importers/scenefile"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/gpu/null"
	"aego/project"
	"aego/render/texture"
	"aego/runtime/bundle"
	"aego/runtime/player"
	"aego/vfs"
)

// Export both runtime data layouts and mount their real production artifacts.
// The original platformer frame IDs are intentionally not rewritten.
func TestLinkedPlatformerArtExportDesktopAndWeb(t *testing.T) {
	p, err := project.Create(t.TempDir(), "Linked Player", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"assets/surfaces/runner.png", "assets/surfaces/runner.png.meta", "assets/surfaces/runner_n.png", "assets/surfaces/runner_n.png.meta", "assets/anim/player_run.aeanim", "assets/anim/player_run.aeanim.meta"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "examples", "platformer", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		writeMaterialExportSource(t, p.Root, name, string(data))
	}
	const scenePath, pngPath, artPath = "scenes/main.aescene", "assets/surfaces/runner.png", "assets/surfaces/runner.aeart"
	writeMaterialExportSource(t, p.Root, scenePath, `scene version=1 {}`)
	reg := pipeline.NewImporterRegistry()
	for _, importer := range []pipeline.Importer{scenefile.NewScene(), artsource.NewTexture(), artsource.New(), animclip.New()} {
		if err := reg.Register(importer); err != nil {
			t.Fatal(err)
		}
	}
	pl, err := pipeline.Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	if b, err := pl.ImportAll(); err != nil || b == nil || len(b.Failed) != 0 || len(b.Retry) != 0 {
		t.Fatalf("platformer import: batch=%+v err=%v", b, err)
	}
	root, ok := pl.Lookup(pngPath)
	if !ok {
		t.Fatal("platformer PNG was not indexed")
	}
	clip, ok := pl.Lookup("assets/anim/player_run.aeanim")
	if !ok {
		t.Fatal("platformer animation was not indexed")
	}
	meta, ok := pl.MetaOf(root)
	if !ok {
		t.Fatal("platformer PNG metadata is unavailable")
	}
	original, err := pl.SourceBytes(pngPath)
	if err != nil || original == nil {
		t.Fatalf("platformer PNG source: %v", err)
	}
	metaBefore, err := pl.SourceBytes(pngPath + pipeline.MetaSuffix)
	if err != nil || metaBefore == nil {
		t.Fatalf("platformer PNG sidecar: %v", err)
	}
	drawing, mapping, err := artsource.ImportTextureDocument(&pipeline.ImportContext{ID: root, Path: pngPath, Source: original, Meta: &meta, Read: pl.SourceBytes, SourcePath: pl.PathOf, Lookup: pl.Lookup})
	if err != nil {
		t.Fatal(err)
	}
	defer drawing.Close()
	if len(mapping) != 8 {
		t.Fatalf("platformer frame mapping has %d frames, want 8", len(mapping))
	}
	session, err := art.NewSession(drawing, art.NewJournal(art.DefaultLimits()))
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Select("", mapping[3].Frame, art.Albedo); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 8, Y: 8}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, G: 32, B: 200, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	if err := session.Select("", mapping[3].Frame, art.Emission); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 8, Y: 8}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	encoded, err := drawing.Encode()
	if err != nil {
		t.Fatal(err)
	}
	artMeta, err := pl.NewSourceMeta(artPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := artsource.SetTextureLink(&meta, artMeta.ID, original, mapping); err != nil {
		t.Fatal(err)
	}
	sceneBefore, err := pl.SourceBytes(scenePath)
	if err != nil || sceneBefore == nil {
		t.Fatalf("scene source: %v", err)
	}
	sceneAfter := []byte(fmt.Sprintf(`scene version=1 { node "11111111111111111111111111111111" name="Player" { component "engine.SpriteRenderer" version=1 { sprite=asset(%q) } component "animation.AnimationPlayer" version=1 { clip=asset(%q) } } }`, mapping[3].Asset.String(), clip.String()))
	if b, err := pl.ApplySources([]pipeline.SourceEdit{
		{Path: artPath, After: encoded}, {Path: artPath + pipeline.MetaSuffix, After: pipeline.WriteMeta(artMeta)},
		{Path: pngPath + pipeline.MetaSuffix, Before: metaBefore, After: pipeline.WriteMeta(&meta)}, {Path: scenePath, Before: sceneBefore, After: sceneAfter},
	}, false); err != nil || b == nil || len(b.Failed) != 0 || len(b.Retry) != 0 {
		t.Fatalf("linked source save: batch=%+v err=%v", b, err)
	}
	p.MainScene, ok = pl.Lookup(scenePath)
	if !ok {
		t.Fatal("authored scene was not indexed")
	}
	snap := Take(pl)
	if ds := Check(p, snap, bundle.Release, false); Blocking(ds) != 0 {
		t.Fatalf("linked art export validation: %+v", ds)
	}
	entry, ok := snap.Index().Lookup(root)
	if !ok {
		t.Fatal("snapshot lost the PNG AssetID")
	}
	prepared, ok := entry.Artifact(asset.ArtTexture)
	if !ok {
		t.Fatal("snapshot has no prepared PNG texture")
	}
	want, err := snap.Artifacts().ReadFile(prepared.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []Target{{OS: "linux", Arch: "amd64"}, webTarget} {
		t.Run(target.String(), func(t *testing.T) {
			out := t.TempDir()
			job := job{p: p, snap: snap, t: target, profile: bundle.Release, rep: &Report{}, l: layoutFor(target, out, p)}
			if err := job.data(); err != nil {
				t.Fatal(err)
			}
			var game *bundle.Data
			var files *vfs.VFS
			if target.Web() {
				payload := &player.Bundle{Files: map[string][]byte{}}
				payload.Manifest, err = os.ReadFile(filepath.Join(job.l.data, bundle.ManifestFile))
				if err != nil {
					t.Fatal(err)
				}
				for _, file := range job.files {
					data, err := os.ReadFile(filepath.Join(job.l.data, filepath.FromSlash(file.Path)))
					if err != nil {
						t.Fatal(err)
					}
					payload.Files[file.Path] = data
				}
				game, files, err = player.MountBytes(payload)
			} else {
				game, files, err = player.Mount(job.l.data)
			}
			if err != nil {
				t.Fatal(err)
			}
			runtimeEntry, ok := game.Index.Lookup(root)
			if !ok {
				t.Fatal("export discarded the existing PNG AssetID")
			}
			runtimeTexture, ok := runtimeEntry.Artifact(asset.ArtTexture)
			if !ok {
				t.Fatal("export lost the PNG texture artifact")
			}
			got, err := files.ReadFile(runtimeTexture.Path)
			if err != nil || !bytes.Equal(want, got) {
				t.Fatal("production export changed the prepared art pixels", err)
			}
			dev := null.New()
			defer dev.Close()
			loaded := loader.New(dev, texture.NewUploader(dev), log.Nop())
			defer loaded.Close()
			if err := loaded.Mount(game.Index, files); err != nil {
				t.Fatal(err)
			}
			for _, link := range mapping {
				if _, ok := loaded.Texture(link.Asset); !ok {
					t.Fatal("legacy frame unavailable in production export")
				}
				if _, _, ok := loaded.NormalFor(link.Asset); !ok {
					t.Fatal("normal surface unavailable in production export")
				}
			}
			if id, _, ok := loaded.EmissionFor(mapping[3].Asset); !ok || id == (ids.AssetID{}) {
				t.Fatal("authored emission unavailable in production export")
			}
			if _, ok := loaded.Anim(clip); !ok {
				t.Fatal("original runtime clip unavailable after linked source export")
			}
		})
	}
}
