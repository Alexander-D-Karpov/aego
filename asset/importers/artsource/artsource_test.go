package artsource

import (
	"aego/art"
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"context"
	"testing"
)

func testSource(t *testing.T) (*pipeline.ImportContext, *art.Session) {
	t.Helper()
	d, err := art.New(16, 16, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	s, err := art.NewSession(d, art.NewJournal(art.DefaultLimits()))
	if err != nil {
		t.Fatal(err)
	}
	root, err := ids.ParseAssetID("1234567890abcdef1234567890abcdef")
	if err != nil {
		t.Fatal(err)
	}
	data, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return &pipeline.ImportContext{ID: root, Path: "assets/hero.aeart", Source: data,
		Meta: &pipeline.Meta{ID: root, Importer: "artsource", Version: Version, Settings: asset.TextureDefaults()}}, s
}

func TestImportUsesExistingRuntimeArtifacts(t *testing.T) {
	ctx, session := testSource(t)
	defer session.Document.Close()
	_, err := session.Execute(context.Background(), art.Operation{Kind: "duplicate-frame", Linked: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx.Source, err = session.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	result, err := New().Import(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != asset.ArtTexture {
		t.Fatalf("root kind: %s", result.Kind)
	}
	textures, regions, clips := 0, 0, 0
	for _, a := range result.Artifacts {
		if a.Kind == asset.ArtTexture {
			textures++
		}
	}
	for _, sub := range result.Subs {
		if sub.Kind == asset.ArtSpriteRegion {
			regions++
		}
		if sub.Kind == asset.ArtAnim {
			clips++
			if len(sub.Deps) == 0 {
				t.Fatal("animation has no frame dependencies")
			}
		}
	}
	if textures != 1 || regions != 2 || clips != 1 {
		t.Fatalf("textures=%d regions=%d clips=%d", textures, regions, clips)
	}
}

func TestFrameIdentitySurvivesDurationChange(t *testing.T) {
	ctx, session := testSource(t)
	defer session.Document.Close()
	before, err := New().Import(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = session.Execute(context.Background(), art.Operation{Kind: "duration", DurationMS: 230}); err != nil {
		t.Fatal(err)
	}
	ctx.Source, err = session.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	after, err := New().Import(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Subs) != len(after.Subs) {
		t.Fatal("subasset count changed")
	}
	for i := range before.Subs {
		if before.Subs[i].ID != after.Subs[i].ID {
			t.Fatal("stable identity changed")
		}
	}
	other, _ := ids.ParseAssetID("2234567890abcdef1234567890abcdef")
	if StableID(ctx.ID, "animation") == StableID(other, "animation") {
		t.Fatal("duplicated source reused IDs")
	}
}
