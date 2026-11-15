package edit

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine"
	"aego/format/aetext"
	inputmodule "aego/modules/input"
	"aego/modules/render2d"
	"aego/project"
	"aego/render/texture"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/sdk"
)

// This in-process registration is the same data contract as the canonical
// example's native/module.go; no second editor executable is needed by CI.
type platformerEnemyStats struct {
	Speed    float32     `aego:"speed,min=0,max=10"`
	Cooldown float32     `aego:"cooldown,min=0,max=5"`
	Damage   int64       `aego:"damage,min=0"`
	Tint     mathx.Color `aego:"tint"`
}

type platformerAuthoringModule struct{}

func (platformerAuthoringModule) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "mygame", Version: "0.1.0"}
}

func (platformerAuthoringModule) Register(r *sdk.Registry) error {
	if err := sdk.DataType[platformerEnemyStats](r, sdk.Name("mygame.EnemyStats")).Err(); err != nil {
		return err
	}
	sdk.Live[platformerEnemyStats](r, "speed")
	return nil
}

func platformerAuthoringProject(t *testing.T) *project.Project {
	t.Helper()
	source := filepath.Join("..", "..", "examples", "platformer")
	root := t.TempDir()
	if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch rel {
			case ".aego", "native", "exports":
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(root, rel), 0755)
		}
		if rel == "go.mod" || rel == "go.sum" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, rel), data, 0644)
	}); err != nil {
		t.Fatal(err)
	}
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	p.Native = false // supplied by platformerAuthoringModule in this process
	return p
}

func TestPlatformerMusicImportsWithoutPlaying(t *testing.T) {
	p := platformerAuthoringProject(t)
	oldPath := filepath.Join(p.Root, "assets/audio/music.ogg")
	old, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, headless(), platformerAuthoringModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Import and inspect only: do not start Play or trigger audio.
	const newPath = "assets/audio/glorious-morning.ogg"
	id, ok := s.Pipeline.Lookup(newPath)
	if !ok || id.String() != "173c7db1e5dc5922da00f5427536495d" {
		t.Fatalf("music asset ID = %s, present=%v", id, ok)
	}
	batch, err := s.Pipeline.Reimport(id)
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("music import failed or needs a retry: %+v, %v", batch, err)
	}
	entry, ok := s.Pipeline.Index().Lookup(id)
	if !ok {
		t.Fatal("music is absent from the imported index")
	}
	artifact, ok := entry.Artifact(asset.ArtAudio)
	if !ok {
		t.Fatal("music has no audio artifact")
	}
	raw, err := s.Pipeline.Artifacts().ReadFile(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	audio, err := asset.DecodeAudio(raw)
	if err != nil || audio.Format != asset.AudioOgg || audio.Rate != 44100 || audio.Channels != 2 || !audio.Stream || audio.Bus != "music" {
		t.Fatalf("unexpected imported music: %+v, %v", audio, err)
	}
	scene, err := os.ReadFile(filepath.Join(p.Root, "scenes/main.aescene"))
	if err != nil || !bytes.Contains(scene, []byte(`Theme=asset("173c7db1e5dc5922da00f5427536495d")`)) {
		t.Fatalf("platformer does not reference the new music: %v", err)
	}
	after, err := os.ReadFile(oldPath)
	if err != nil || !bytes.Equal(old, after) {
		t.Fatalf("original music was changed: %v", err)
	}
}

// gpu/null proves the canonical authoring and live resource transaction. The
// actual displayed Lit pixels are covered separately by the GL/browser route.
func TestPlatformerAssetSaveAndUndoDuringPlay(t *testing.T) {
	p := platformerAuthoringProject(t)
	s, err := Open(p, headless(), platformerAuthoringModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ed := s.App.Context()
	doc, err := ed.Docs.Open("scenes/main.aescene")
	if err != nil {
		t.Fatal(err)
	}
	s.Step()
	beforeScene := sceneio.Save(doc.Scene())
	beforePrefab, err := os.ReadFile(filepath.Join(p.Root, "prefabs", "enemy.aeprefab"))
	if err != nil {
		t.Fatal(err)
	}
	const albedoPath = "assets/surfaces/platform.png"
	albedoID, ok := ed.Pipeline.Lookup(albedoPath)
	if !ok {
		t.Fatal("canonical platform texture missing")
	}
	ed.Open(context.AssetDocumentAt(albedoPath))
	albedoDoc := ed.Docs.AssetDocument(albedoID)
	if albedoDoc == nil || albedoDoc.Metadata == nil || albedoDoc.Path != albedoPath {
		t.Fatal("existing platform image did not open through the shared document route")
	}
	meta, err := asset.ParseSourceMeta(albedoDoc.Metadata.Code.Bytes(), albedoDoc.Metadata.Path)
	if err != nil {
		t.Fatal(err)
	}
	heightID, explicit, err := asset.TextureChannel(meta, albedoID, "height")
	if err != nil || !explicit || heightID.IsZero() {
		t.Fatal("canonical platform is not connected to an explicit Height channel", err)
	}
	d, err := ed.OpenAsset(heightID)
	if err != nil || d == nil || d.Path != "assets/surfaces/platform_height.png" || d.Metadata == nil {
		t.Fatal("linked height channel did not open its shared source document", err)
	}
	if shared, err := ed.DocumentForAsset(heightID); err != nil || shared != d || d.Source != nil {
		t.Fatal("Inspector and linked image do not share one metadata journal", err)
	}
	beforeSource, err := os.ReadFile(ed.Docs.Abs(d.Path))
	if err != nil {
		t.Fatal(err)
	}
	readNormal := func() []byte {
		t.Helper()
		e, ok := ed.Loader.Index().Lookup(heightID)
		if !ok {
			t.Fatal("height runtime entry missing")
		}
		if a, region := e.Artifact(asset.ArtSpriteRegion); region {
			raw, err := ed.Loader.Artifacts().ReadFile(a.Path)
			if err != nil {
				t.Fatal(err)
			}
			mapping, err := asset.DecodeRegion(raw)
			if err != nil {
				t.Fatal(err)
			}
			e, ok = ed.Loader.Index().Lookup(mapping.Page)
			if !ok {
				t.Fatal("generated normal atlas page missing")
			}
		}
		a, ok := e.Artifact(asset.ArtTexture)
		if !ok {
			a, ok = e.Artifact(asset.ArtAtlasPage)
		}
		if !ok {
			t.Fatal("generated normal texture artifact missing")
		}
		raw, err := ed.Loader.Artifacts().ReadFile(a.Path)
		if err != nil {
			t.Fatal(err)
		}
		normal, err := asset.DecodeTexture(raw)
		if err != nil || normal.Semantic != asset.TextureNormal {
			t.Fatal("height settings did not generate a linear normal artifact", err)
		}
		return bytes.Clone(normal.Pixels)
	}
	initial := readNormal()
	_, initialRegion, ok := ed.Loader.NormalFor(albedoID)
	if !ok {
		t.Fatal("canonical platform has no effective generated normal")
	}
	pc := s.App.Play()
	if err := pc.Start(doc); err != nil {
		t.Fatal(err)
	}
	playWorld := pc.World()
	pc.Tick(1.0 / 60)
	assertConsumers := func(want texture.Region) {
		t.Helper()
		if pc.World() != playWorld {
			t.Fatal("asset authoring stopped or replaced the Play world")
		}
		s.Engine.RunPhase(playWorld, engine.PhaseRenderPrepare, engine.ScopePlay, 0, 1)
		s.Engine.RunPhase(playWorld, engine.PhaseRender, engine.ScopePlay, 0, 1)
		r := frame.Of(&engine.Context{World: playWorld})
		albedo, ok := ed.Loader.Texture(albedoID)
		if !ok || r == nil {
			t.Fatal("live Play extraction has no platform source")
		}
		consumers := 0
		for i := range r.Views() {
			for _, draw := range r.Bucket(i).Sprites {
				if draw.Region != albedo {
					continue
				}
				consumers++
				if !draw.Lighting || draw.Normal != want || !draw.HasLinear {
					t.Fatal("canonical Play consumer retained a stale normal or basis")
				}
			}
		}
		if consumers < 2 {
			t.Fatalf("the actual level must share the platform texture between multiple receivers: got=%d views=%d stats=%+v albedo=%+v", consumers, len(r.Views()), r.Stats(), albedo)
		}
	}
	assertConsumers(initialRegion)
	env, err := ed.TextureAuthoringEnv(heightID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := document.ApplyTextureMutation(d.Metadata, env, mutate.Tx{Label: "Platform height strength", Ops: []mutate.Op{mutate.TextureSetting("height_strength", aetext.Float32(9))}}, false); err != nil {
		t.Fatal(err)
	}
	panelID := "render2d.image/" + d.Asset.String()
	run := func(id string) {
		t.Helper()
		s.App.Workspace().Focus(panelID)
		if err := ed.Commands.Run(id); err != nil {
			t.Fatal(id, err)
		}
	}
	run(command.FileSave)
	s.Step()
	saved := readNormal()
	_, savedRegion, resolved := ed.Loader.NormalFor(albedoID)
	if bytes.Equal(initial, saved) || d.Dirty() || !resolved || savedRegion.Texture == initialRegion.Texture {
		t.Fatal("Save during Play did not publish changed normal pixels and a new shared GPU resource")
	}
	assertConsumers(savedRegion)
	diskSaved, err := os.ReadFile(ed.Docs.Abs(d.Metadata.Path))
	if err != nil {
		t.Fatal(err)
	}
	awaitHistory := func(id string, pixels []byte, dirty bool) {
		t.Helper()
		previous := d.WorkingPreview.Applied
		run(id)
		deadline := time.Now().Add(8 * time.Second)
		for d.WorkingPreview.Applied == previous && d.WorkingPreview.Error == "" && time.Now().Before(deadline) {
			s.Step()
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		if d.WorkingPreview.Applied == previous || d.WorkingPreview.Pending || d.WorkingPreview.Error != "" || d.Dirty() != dirty || !bytes.Equal(pixels, readNormal()) {
			t.Fatalf("canonical live asset history failed: %s, %+v, dirty=%v", id, d.WorkingPreview, d.Dirty())
		}
		current, err := os.ReadFile(ed.Docs.Abs(d.Metadata.Path))
		if err != nil || !bytes.Equal(current, diskSaved) {
			t.Fatal("live Undo/Redo silently wrote the saved source metadata", err)
		}
		_, normal, ok := ed.Loader.NormalFor(albedoID)
		if !ok {
			t.Fatal("live Undo/Redo lost the saved source association")
		}
		assertConsumers(normal)
	}
	awaitHistory(command.EditUndo, initial, true)
	awaitHistory(command.EditRedo, saved, false)

	// Exercise the real character script and animation. Each emitted draw must
	// use the current stable frame's own normal region, with FlipX carried once.
	player := pc.Scene().Find(pc.Scene().Root(), "Player")
	inputs := inputmodule.Of(playWorld)
	if player.IsZero() || inputs == nil {
		t.Fatal("canonical Player or Input Actions missing")
	}
	frames := map[ids.AssetID]bool{}
	for _, move := range []float32{1, -1} {
		if err := inputs.Inject(0, "Move", move); err != nil {
			t.Fatal(err)
		}
		for range 30 {
			pc.Tick(1.0 / 60)
			s.Engine.RunPhase(playWorld, engine.PhaseRenderPrepare, engine.ScopePlay, 0, 1)
			s.Engine.RunPhase(playWorld, engine.PhaseRender, engine.ScopePlay, 0, 1)
			sr, ok := scene.Get[render2d.SpriteRenderer](pc.Scene(), player)
			if !ok || sr.FlipX != (move < 0) {
				t.Fatal("real player movement did not update FlipX")
			}
			frames[sr.Sprite.ID] = true
			albedo, ok := sr.Sprite.Region(ed.Loader)
			normalID, normal, resolved := ed.Loader.NormalFor(sr.Sprite.ID)
			if !ok || !resolved || normalID.IsZero() || normal.Size() != albedo.Size() {
				t.Fatal("animated runner frame lost its source-aligned normal region")
			}
			r := frame.Of(&engine.Context{World: playWorld})
			found := false
			for i := range r.Views() {
				for _, draw := range r.Bucket(i).Sprites {
					if draw.Region == albedo && draw.Order == int16(sr.Order) && draw.Tint == sr.Tint {
						found = true
						if draw.FlipX != sr.FlipX || draw.Normal != normal || !draw.Lighting || !draw.HasLinear {
							t.Fatal("FlipX or the normal region changed between the real animation and extraction")
						}
					}
				}
			}
			if !found {
				t.Fatal("live player is absent from the actual Play draw stream")
			}
		}
	}
	if len(frames) < 2 {
		t.Fatal("runner animation did not advance through distinct stable frame IDs")
	}
	pc.Stop()
	if doc.Dirty() || !bytes.Equal(beforeScene, sceneio.Save(doc.Scene())) {
		t.Fatal("asset Save/Undo or gameplay presentation modified EditScene")
	}
	if now, err := os.ReadFile(ed.Docs.Abs(d.Path)); err != nil || !bytes.Equal(now, beforeSource) {
		t.Fatal("height metadata authoring edited the source image pixels", err)
	}
	if now, err := os.ReadFile(filepath.Join(p.Root, "prefabs", "enemy.aeprefab")); err != nil || !bytes.Equal(now, beforePrefab) {
		t.Fatal("live asset authoring changed a gameplay prefab", err)
	}
}
