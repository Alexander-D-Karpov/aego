package edit

import (
	"bytes"
	"context"
	"errors"
	"image"
	"os"
	"runtime"
	"testing"
	"time"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/artedit"
	"aego/editor/core/command"
	"aego/editor/core/document"
	"aego/engine"
	inputmodule "aego/modules/input"
	"aego/modules/render2d"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
)

func TestPlatformerEditSourceArtHeadless(t *testing.T) {
	platformerSourceArtWorkflow(t, headless(), nil, nil, nil)
}

// Opening and the first-save prompt must not create a source or change either
// the original image or its metadata before explicit confirmation.
func assertPlayerLinkPending(t *testing.T, s *Session, pngBefore, metaBefore []byte) {
	t.Helper()
	for _, check := range []struct {
		path string
		want []byte
	}{
		{"assets/surfaces/runner.png", pngBefore},
		{"assets/surfaces/runner.png.meta", metaBefore},
	} {
		got, err := s.Pipeline.SourceBytes(check.path)
		if err != nil || !bytes.Equal(got, check.want) {
			t.Fatalf("link prompt modified %s: %v", check.path, err)
		}
	}
	for _, path := range []string{"assets/surfaces/runner.aeart", "assets/surfaces/runner.aeart.meta"} {
		if _, err := os.Stat(s.App.Context().Docs.Abs(path)); !os.IsNotExist(err) {
			t.Fatalf("link prompt created %s before confirmation: %v", path, err)
		}
	}
}

// Native callers supply real Inspector and canvas SDL interactions. This
// shared test always uses the canonical platformer scene, imported PNG and
// Animator, rather than replacing Player with a synthetic art-only fixture.
func platformerSourceArtWorkflow(t *testing.T, cfg Config, open func(*testing.T, *Session, ids.AssetID), paint func(*testing.T, *Session, string, *artedit.Document), observe func(*testing.T, *Session, string, string)) {
	t.Helper()
	p := platformerAuthoringProject(t)
	s, err := Open(p, cfg, platformerAuthoringModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	if !s.Step() {
		t.Fatal("editor startup stopped")
	}
	ed := s.App.Context()
	sceneDoc, err := ed.Docs.Open("scenes/main.aescene")
	if err != nil {
		t.Fatal(err)
	}
	s.Step()
	sc := sceneDoc.Scene()
	player := sc.Find(sc.Root(), "Player")
	sprite, ok := scene.Get[render2d.SpriteRenderer](sc, player)
	if !ok || sprite.Sprite.ID.IsZero() {
		t.Fatal("canonical Player SpriteRenderer missing")
	}
	ed.Select(sc.ID(player))
	playerFrame := sprite.Sprite.ID
	sceneBefore := sceneio.Save(sc)
	const pngPath = "assets/surfaces/runner.png"
	const artPath = "assets/surfaces/runner.aeart"
	root, ok := ed.Pipeline.Lookup(pngPath)
	if !ok {
		t.Fatal("canonical runner source missing")
	}
	originalPNG, err := ed.Pipeline.SourceBytes(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	originalMeta, err := ed.Pipeline.SourceBytes(pngPath + ".meta")
	if err != nil || len(originalMeta) == 0 {
		t.Fatal("canonical runner metadata missing", err)
	}
	metaBefore, _ := ed.Pipeline.MetaOf(root)
	readPixels := func() []byte {
		t.Helper()
		e, ok := ed.Loader.Index().Lookup(root)
		if !ok {
			t.Fatal("original PNG lost its runtime identity")
		}
		a, ok := e.Artifact(asset.ArtTexture)
		if !ok {
			t.Fatal("runner texture is unavailable")
		}
		encoded, err := ed.Loader.Artifacts().ReadFile(a.Path)
		if err != nil {
			t.Fatal(err)
		}
		texture, err := asset.DecodeTexture(encoded)
		if err != nil {
			t.Fatal(err)
		}
		return bytes.Clone(texture.Pixels)
	}
	initial := readPixels()
	previousBeforeSwap := ed.BeforeSwap
	batches := 0
	ed.BeforeSwap = func(batch *pipeline.Batch) func() {
		batches++
		if len(batch.Failed) != 0 || len(batch.Retry) != 0 {
			t.Errorf("link import batch failed or requires retry: failed=%v retry=%v", batch.Failed, batch.Retry)
		}
		if previousBeforeSwap != nil {
			return previousBeforeSwap(batch)
		}
		return nil
	}
	if open != nil {
		open(t, s, playerFrame)
	} else {
		if err := ed.EditSourceArt(playerFrame); err != nil {
			t.Fatal(err)
		}
		if questions := ed.TakeQuestions(); len(questions) != 0 {
			t.Fatal("opening the PNG unexpectedly requested conversion")
		}
		assertPlayerLinkPending(t, s, originalPNG, originalMeta)
		draft := ed.Docs.AssetDocument(root)
		if draft == nil {
			t.Fatal("opening PNG did not create a shared draft")
		}
		if err := ed.SaveAsset(draft); !errors.Is(err, document.ErrSavePending) {
			t.Fatalf("first Save did not wait for confirmation: %v", err)
		}
		questions := ed.TakeQuestions()
		if len(questions) != 1 || questions[0].Submit == nil {
			t.Fatal("first Save did not show the linked source warning and path")
		}
		assertPlayerLinkPending(t, s, originalPNG, originalMeta)
		questions[0].Submit([]string{artPath})
	}
	ed.BeforeSwap = previousBeforeSwap
	if batches == 0 {
		t.Fatal("link creation did not publish an import batch")
	}
	artID, ok := ed.Pipeline.Lookup(artPath)
	if !ok {
		notice, _ := ed.LastNotice()
		t.Fatalf("Edit Sprite did not create the confirmed authoring source: %+v", notice)
	}
	for _, id := range []ids.AssetID{root, artID, playerFrame} {
		if err := ed.Pipeline.VerifyCurrent(id); err != nil {
			t.Fatalf("linked source or frame is not current: %v", err)
		}
	}
	doc := ed.Docs.AssetDocument(artID)
	if doc == nil {
		t.Fatal("Edit Sprite did not open an art document")
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok || len(model.Session.Info().Document.Frames) != 8 {
		t.Fatal("runner spritesheet did not open as eight Art Studio frames")
	}
	panelID := "render2d.art/" + artID.String()
	if !s.App.Workspace().Visible(panelID) || !bytes.Equal(initial, readPixels()) {
		t.Fatal("Edit Sprite lost source pixels or did not open a dockable authoring tab")
	}
	linkedMeta, _ := ed.Pipeline.MetaOf(root)
	link, err := artsource.TextureLinkOf(&linkedMeta)
	if err != nil || link == nil || len(link.Frames) != 8 || len(linkedMeta.Subs) != len(metaBefore.Subs) {
		t.Fatalf("linked runner source mapping invalid: %v", err)
	}
	for i, sub := range linkedMeta.Subs {
		if sub.ID != metaBefore.Subs[i].ID || sub.Rect != metaBefore.Subs[i].Rect || sub.EffectivePivot() != metaBefore.Subs[i].EffectivePivot() {
			t.Fatal("Edit Sprite changed a legacy frame ID, region or pivot")
		}
	}
	if model.Session.Frame != link.Frames[0].Frame {
		t.Fatal("Edit Sprite did not select the Player's current region")
	}
	if observe != nil {
		observe(t, s, panelID, "scene-before")
	}
	if paint != nil {
		paint(t, s, panelID, model)
	} else if _, err := model.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 8, Y: 8}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, B: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	if !doc.Dirty() || model.History.Cursor() != 1 {
		t.Fatal("the paint gesture is not one undoable authoring operation")
	}
	waitPreview := func(previous uint64) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for doc.WorkingPreview.Applied == previous && doc.WorkingPreview.Error == "" && time.Now().Before(deadline) {
			if !s.Step() {
				t.Fatal("editor stopped during runner preview")
			}
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		if doc.WorkingPreview.Applied == previous || doc.WorkingPreview.Pending || doc.WorkingPreview.Error != "" {
			t.Fatalf("runner preview failed: %+v", doc.WorkingPreview)
		}
	}
	waitPreview(0)
	painted := readPixels()
	if bytes.Equal(initial, painted) || !ed.WorkingSourceCurrent(doc, model.PreviewRevision()) {
		t.Fatal("unsaved frame pixels did not reach the scene's shared asset loader")
	}
	if observe != nil {
		observe(t, s, panelID, "scene-painted")
	}
	run := func(id string) {
		t.Helper()
		s.App.Workspace().Focus(panelID)
		if err := ed.Commands.Run(id); err != nil {
			t.Fatal(id, err)
		}
	}
	previous := doc.WorkingPreview.Applied
	run(command.EditUndo)
	waitPreview(previous)
	if !bytes.Equal(initial, readPixels()) {
		t.Fatal("common Undo did not restore the Player source pixels")
	}
	if observe != nil {
		observe(t, s, panelID, "scene-undo")
	}
	previous = doc.WorkingPreview.Applied
	run(command.EditRedo)
	waitPreview(previous)
	if !bytes.Equal(painted, readPixels()) {
		t.Fatal("common Redo did not restore the painted Player pixels")
	}
	if observe != nil {
		observe(t, s, panelID, "scene-redo")
	}
	shape, ok := ed.Loader.SpriteShadow(playerFrame)
	if !ok || len(shape.Parts) == 0 {
		t.Fatal("Player's existing alpha silhouette missing")
	}
	previous = doc.WorkingPreview.Applied
	if _, err := model.Execute(context.Background(), art.Operation{Kind: "clear"}); err != nil {
		t.Fatal(err)
	}
	waitPreview(previous)
	shape, ok = ed.Loader.SpriteShadow(playerFrame)
	if !ok || len(shape.Parts) != 0 {
		t.Fatal("editing alpha did not rebuild the Player frame's real silhouette")
	}
	if observe != nil {
		observe(t, s, panelID, "scene-cleared-alpha")
	}
	previous = doc.WorkingPreview.Applied
	run(command.EditUndo)
	waitPreview(previous)
	shape, ok = ed.Loader.SpriteShadow(playerFrame)
	if !ok || len(shape.Parts) == 0 {
		t.Fatal("Undo did not restore Player's silhouette")
	}
	if err := model.Session.Select("", model.Session.Frame, art.Emission); err != nil {
		t.Fatal(err)
	}
	previous = doc.WorkingPreview.Applied
	if _, err := model.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 8, Y: 8}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, G: 170, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	waitPreview(previous)
	if id, _, ok := ed.Loader.EmissionFor(playerFrame); !ok || id.IsZero() {
		t.Fatal("dirty emission did not become a real runtime surface for Player")
	}
	if err := model.Session.Select("", model.Session.Frame, art.Albedo); err != nil {
		t.Fatal(err)
	}
	// The existing Animator and character script must continue to reference the
	// original PNG frame IDs, with real normals and FlipX in the draw stream.
	pc := s.App.Play()
	if err := pc.Start(sceneDoc); err != nil {
		t.Fatal(err)
	}
	playWorld := pc.World()
	inputs := inputmodule.Of(playWorld)
	playPlayer := pc.Scene().Find(pc.Scene().Root(), "Player")
	if inputs == nil || playPlayer.IsZero() {
		t.Fatal("canonical Player runtime was not started")
	}
	seen := map[ids.AssetID]bool{}
	for _, move := range []float32{1, -1} {
		if err := inputs.Inject(0, "Move", move); err != nil {
			t.Fatal(err)
		}
		for range 30 {
			pc.Tick(1.0 / 60)
			s.Engine.RunPhase(playWorld, engine.PhaseRenderPrepare, engine.ScopePlay, 0, 1)
			s.Engine.RunPhase(playWorld, engine.PhaseRender, engine.ScopePlay, 0, 1)
			sr, ok := scene.Get[render2d.SpriteRenderer](pc.Scene(), playPlayer)
			if !ok || sr.FlipX != (move < 0) {
				t.Fatal("Player script did not drive FlipX")
			}
			seen[sr.Sprite.ID] = true
			albedo, ok := sr.Sprite.Region(ed.Loader)
			_, normal, normalOK := ed.Loader.NormalFor(sr.Sprite.ID)
			if !ok || !normalOK || normal.Size() != albedo.Size() {
				t.Fatal("authored Player frame lost its aligned normal surface")
			}
			r := frame.Of(&engine.Context{World: playWorld})
			found := false
			for i := range r.Views() {
				for _, draw := range r.Bucket(i).Sprites {
					if draw.Region == albedo && draw.Order == int16(sr.Order) && draw.Tint == sr.Tint {
						found = true
						if draw.FlipX != sr.FlipX || draw.Normal != normal || !draw.Lighting || !draw.HasLinear {
							t.Fatal("authored Player is not rendered through the real Lit/FlipX path")
						}
					}
				}
			}
			if !found {
				t.Fatal("Player is absent from Game View draw stream")
			}
		}
	}
	if len(seen) < 2 {
		t.Fatal("real Animator did not advance authored frames")
	}
	for id := range seen {
		if _, ok := metaBefore.Frame(id); !ok {
			t.Fatal("runtime animation references changed during source editing")
		}
	}
	if observe != nil {
		// Let the real Animator select the authored frame again, then hold the
		// existing Play world while the native UI captures actual Game View
		// pixels. Do not replace its SpriteRef, camera, transform or collider.
		if err := inputs.Inject(0, "Move", 1); err != nil {
			t.Fatal(err)
		}
		found := false
		for range 120 {
			pc.Tick(1.0 / 60)
			sr, ok := scene.Get[render2d.SpriteRenderer](pc.Scene(), playPlayer)
			if ok && sr.Sprite.ID == playerFrame {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("canonical Animator did not return to the painted Player frame")
		}
		pc.Pause()
		observe(t, s, panelID, "game-painted")
		// The two current changes are albedo and emission. Undo both through
		// the same authoring command, compare the Player's paused GPU region,
		// then restore both before saving the intended authored source.
		for range 2 {
			previous = doc.WorkingPreview.Applied
			run(command.EditUndo)
			waitPreview(previous)
		}
		observe(t, s, panelID, "game-undo")
		for range 2 {
			previous = doc.WorkingPreview.Applied
			run(command.EditRedo)
			waitPreview(previous)
		}
		observe(t, s, panelID, "game-redo")
	}
	pc.Stop()
	run(command.FileSave)
	if doc.Dirty() {
		t.Fatal("Save did not save the canonical dirty source")
	}
	if sceneDoc.Dirty() || !bytes.Equal(sceneBefore, sceneio.Save(sc)) || sprite.Sprite.ID != playerFrame {
		t.Fatal("source authoring changed EditScene, collider properties or AssetRef")
	}
	if png, err := ed.Pipeline.SourceBytes(pngPath); err != nil || !bytes.Equal(png, originalPNG) {
		t.Fatal("source editing overwrote the user's original PNG", err)
	}
	s.Close()
	s, err = Open(p, cfg, platformerAuthoringModule{})
	if err != nil {
		t.Fatal(err)
	}
	s.Step()
	ed = s.App.Context()
	if err := ed.EditSourceArt(playerFrame); err != nil {
		t.Fatal(err)
	}
	doc = ed.Docs.AssetDocument(artID)
	if doc == nil || doc.Dirty() || !bytes.Equal(painted, readPixels()) {
		t.Fatal("editor restart lost saved authored pixels or source ownership")
	}
	model = doc.Binding.Model.(*artedit.Document)
	if len(model.Session.Info().Document.Frames) != 8 || model.Session.Frame != link.Frames[0].Frame {
		t.Fatal("editor restart lost frame identity or selected source frame")
	}
	if id, _, ok := ed.Loader.EmissionFor(playerFrame); !ok || id.IsZero() {
		t.Fatal("editor restart lost the saved emission surface")
	}
	t.Logf("canonical Player: eight legacy frames, paint/Undo/Redo, alpha silhouette, emission, Lit normal/FlipX, actual Animator, source save and editor restart PASS")
}
