package edit

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	gameexport "aego/app/export"
	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/editor/core/command"
	"aego/engine/problems"
	"aego/modules/animation"
	"aego/modules/render2d"
	"aego/scene"
	"aego/scene/sceneio"
)

func TestArtSceneAuthoringWorkflowHeadless(t *testing.T) {
	artSceneAuthoringWorkflow(t, headless(), nil, nil)
}

// The optional input callback drives the panel in native GL tests. The default
// path changes the same editor-owned model without claiming an OS UI test.
func artSceneAuthoringWorkflow(t *testing.T, cfg Config, input func(*testing.T, *Session, string, *artedit.Document), observe func(*testing.T, *Session, string, string)) {
	t.Helper()
	p := newProject(t)
	id := ids.NewAssetID()
	drawing, err := art.New(16, 16, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	firstFrame := drawing.Info().Frames[0].ID
	source, err := drawing.Encode()
	drawing.Close()
	if err != nil {
		t.Fatal(err)
	}
	const artPath = "assets/hero.aeart"
	if err := os.WriteFile(filepath.Join(p.Root, artPath), source, 0644); err != nil {
		t.Fatal(err)
	}
	imp := artsource.New()
	meta := asset.NewSourceMeta(imp.ID(), imp.Version(), imp.Defaults())
	meta.ID = id
	if err := os.WriteFile(filepath.Join(p.Root, artPath+".meta"), asset.MarshalSourceMeta(meta), 0644); err != nil {
		t.Fatal(err)
	}
	const scenePath = "scenes/art-workflow.aescene"
	writeScene(t, p, scenePath, func(sc *scene.Scene) {
		cam := sc.Create(sc.Root(), "Camera")
		*scene.Add[render2d.Camera2D](sc, cam) = render2d.DefaultCamera2D()
		hero := sc.Create(sc.Root(), "Hero")
		sprite := scene.Add[render2d.SpriteRenderer](sc, hero)
		*sprite = render2d.DefaultSpriteRenderer()
		sprite.Sprite.ID, sprite.Size = id, mathx.Vec2{X: 4, Y: 4}
		player := animation.DefaultPlayer()
		player.Clip.ID = artsource.StableID(id, artsource.ClipName(""))
		*scene.Add[animation.AnimationPlayer](sc, hero) = player
	})
	s, err := Open(p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Step() {
		t.Fatal("editor stopped during startup")
	}
	ed, ws := s.App.Context(), s.App.Workspace()
	sceneDoc, err := ed.Docs.Open(scenePath)
	if err != nil {
		t.Fatal(err)
	}
	s.Step()
	beforeScene := sceneio.Save(sceneDoc.Scene())
	hero := sceneDoc.Scene().Find(sceneDoc.Scene().Root(), "Hero")
	sprite, ok := scene.Get[render2d.SpriteRenderer](sceneDoc.Scene(), hero)
	if !ok || sprite.Sprite.ID != id {
		t.Fatal("scene lost its source AssetID")
	}
	readTexture := func() []byte {
		t.Helper()
		entry, ok := ed.Loader.Index().Lookup(id)
		if !ok {
			t.Fatal("loader lost the sprite source")
		}
		a, ok := entry.Artifact(asset.ArtTexture)
		if !ok {
			t.Fatal("sprite has no texture artifact")
		}
		raw, err := ed.Loader.Artifacts().ReadFile(a.Path)
		if err != nil {
			t.Fatal(err)
		}
		texture, err := asset.DecodeTexture(raw)
		if err != nil {
			t.Fatal(err)
		}
		return bytes.Clone(texture.Pixels)
	}
	initialPixels := readTexture()
	doc, err := ed.OpenAsset(id)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok || doc.Metadata == nil {
		t.Fatal("art editor did not bind the existing source and metadata")
	}
	panelID := "render2d.art/" + id.String()
	if !ws.Visible(panelID) {
		t.Fatal("art editor did not open a workspace tab")
	}
	if _, ok := ed.Loader.Index().Lookup(artsource.StableID(id, artsource.ClipName(""))); !ok {
		t.Fatal("existing runtime animation clip was not imported")
	}
	if observe != nil {
		observe(t, s, panelID, "scene-before")
	}
	if input != nil {
		input(t, s, panelID, model)
	} else {
		_, err = model.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 4, Y: 4}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !doc.Dirty() || !doc.CanUndo() {
		t.Fatal("art stroke did not enter the shared history")
	}
	exporter := s.App.AgentExporter()
	if exporter == nil {
		t.Fatal("editor export service is unavailable")
	}
	blocked := false
	for _, diagnostic := range exporter.Check(exporter.Prepare(), "release") {
		if diagnostic.Code == gameexport.CodeDirtySource && diagnostic.Severity == problems.Error && diagnostic.Subject.File == artPath {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("export accepted stale saved pixels while the art document is dirty")
	}
	waitPreview := func(previous uint64) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for doc.WorkingPreview.Applied == previous && doc.WorkingPreview.Error == "" && time.Now().Before(deadline) {
			if !s.Step() {
				t.Fatal("editor stopped while preparing preview")
			}
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		if doc.WorkingPreview.Applied == previous || doc.WorkingPreview.Pending || doc.WorkingPreview.Error != "" {
			t.Fatalf("art preview failed: %+v", doc.WorkingPreview)
		}
	}
	waitPreview(0)
	paintedPixels := readTexture()
	if bytes.Equal(initialPixels, paintedPixels) {
		t.Fatal("unsaved art did not update the scene's asset loader")
	}
	if observe != nil {
		observe(t, s, panelID, "scene-painted")
	}
	if disk, err := os.ReadFile(ed.Docs.Abs(artPath)); err != nil || !bytes.Equal(disk, source) {
		t.Fatal("working preview saved the source without permission")
	}
	if err := s.App.Play().Start(sceneDoc); err != nil {
		t.Fatal(err)
	}
	frameID := artsource.StableID(id, artsource.FrameName(firstFrame, art.Albedo))
	deadline := time.Now().Add(8 * time.Second)
	var runtimeFrame ids.AssetID
	for time.Now().Before(deadline) {
		if !s.Step() {
			t.Fatal("editor stopped during animation playback")
		}
		playScene := s.App.Play().Scene()
		playHero := playScene.Find(playScene.Root(), "Hero")
		if playerSprite, ok := scene.Get[render2d.SpriteRenderer](playScene, playHero); ok {
			runtimeFrame = playerSprite.Sprite.ID
			if runtimeFrame == frameID {
				break
			}
		}
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	if runtimeFrame != frameID {
		t.Fatalf("AnimationPlayer did not assign the authored frame: got %s, want %s", runtimeFrame, frameID)
	}
	if observe != nil {
		observe(t, s, panelID, "game-painted")
	}
	run := func(id string) {
		t.Helper()
		ws.Focus(panelID)
		if err := ed.Commands.Run(id); err != nil {
			t.Fatal(id, err)
		}
	}
	previous := doc.WorkingPreview.Applied
	run(command.EditUndo)
	waitPreview(previous)
	if !bytes.Equal(initialPixels, readTexture()) {
		t.Fatal("Undo during Play did not restore the scene asset")
	}
	if observe != nil {
		observe(t, s, panelID, "game-undo")
	}
	previous = doc.WorkingPreview.Applied
	run(command.EditRedo)
	waitPreview(previous)
	if !bytes.Equal(paintedPixels, readTexture()) {
		t.Fatal("Redo during Play did not restore the working sprite")
	}
	if observe != nil {
		observe(t, s, panelID, "game-redo")
	}
	s.App.Play().Stop()
	run(command.FileSave)
	if doc.Dirty() {
		t.Fatal("generic Save left art dirty")
	}
	for _, diagnostic := range exporter.Check(exporter.Prepare(), "release") {
		if diagnostic.Code == gameexport.CodeDirtySource && diagnostic.Subject.File == artPath {
			t.Fatal("saved art still blocks export as dirty")
		}
	}
	entry, ok := ed.Pipeline.Entry(id)
	if !ok {
		t.Fatal("Save removed the original AssetID")
	}
	artifact, ok := entry.Artifact(asset.ArtTexture)
	if !ok {
		t.Fatal("Save did not persist the source texture")
	}
	encoded, err := ed.Pipeline.Artifacts().ReadFile(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := asset.DecodeTexture(encoded)
	if err != nil || !bytes.Equal(saved.Pixels, paintedPixels) {
		t.Fatalf("saved pipeline artifact differs from the working sprite: %v", err)
	}
	if sceneDoc.Dirty() || !bytes.Equal(sceneio.Save(sceneDoc.Scene()), beforeScene) || sprite.Sprite.ID != id {
		t.Fatal("art editing changed scene content or the sprite reference")
	}

	// Extend the saved drawing without saving again. The existing AnimationPlayer
	// must consume the newly prepared frame and clip through the same loader.
	previous = doc.WorkingPreview.Applied
	secondFrame, err := model.Execute(context.Background(), art.Operation{Kind: "duplicate-frame"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := model.Execute(context.Background(), art.Operation{Kind: "duration", DurationMS: 80}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 6, Y: 6}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	waitPreview(previous)
	secondID := artsource.StableID(id, artsource.FrameName(secondFrame, art.Albedo))
	if _, ok := ed.Loader.Index().Lookup(secondID); !ok {
		t.Fatal("unsaved second frame did not reach the asset loader")
	}
	if _, persisted := ed.Pipeline.PathOf(secondID); persisted {
		t.Fatal("the unsaved frame unexpectedly entered the persistent pipeline")
	}
	if source, ok := ed.SourcePathForAsset(secondID); !ok || source != artPath {
		t.Fatalf("working frame did not resolve to its source: %q, %v", source, ok)
	}
	if opened, err := ed.OpenAsset(secondID); err != nil || opened != doc || model.Session.Frame != secondFrame {
		t.Fatalf("working frame did not open its owning document at the current frame: %v", err)
	}
	previous = doc.WorkingPreview.Applied
	if _, err := model.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 7, Y: 7}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{G: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := ed.SourcePathForAsset(secondID); ok {
		t.Fatal("a stale preview frame was allowed to resolve as current")
	}
	if _, err := ed.OpenAsset(secondID); err == nil {
		t.Fatal("a stale preview frame was opened while its replacement was pending")
	}
	waitPreview(previous)
	if opened, err := ed.OpenAsset(secondID); err != nil || opened != doc || model.Session.Frame != secondFrame {
		t.Fatalf("updated working frame did not open after publication: %v", err)
	}
	if err := s.App.Play().Start(sceneDoc); err != nil {
		t.Fatal(err)
	}
	seen := map[ids.AssetID]bool{}
	deadline = time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) && (!seen[frameID] || !seen[secondID]) {
		if !s.Step() {
			t.Fatal("editor stopped while playing the unsaved animation")
		}
		playScene := s.App.Play().Scene()
		playHero := playScene.Find(playScene.Root(), "Hero")
		if playSprite, ok := scene.Get[render2d.SpriteRenderer](playScene, playHero); ok {
			seen[playSprite.Sprite.ID] = true
		}
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	s.App.Play().Stop()
	if !seen[frameID] || !seen[secondID] {
		t.Fatalf("AnimationPlayer did not play both working frames: %+v", seen)
	}
	if !doc.Dirty() || sceneDoc.Dirty() || !bytes.Equal(sceneio.Save(sceneDoc.Scene()), beforeScene) {
		t.Fatal("working animation was saved implicitly or modified its scene")
	}
}
