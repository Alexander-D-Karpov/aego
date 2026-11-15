package editor

import (
	"bytes"
	stdctx "context"
	"fmt"
	"image"
	"testing"

	"aego/anim"
	"aego/art"
	"aego/asset/importers/artsource"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/format/aetext"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/scene"
)

func TestDuplicateSpriteSnapshotsDirtyArtWithoutChangingOtherConsumers(t *testing.T) {
	ed, root, frames, _ := linkedArtEditor(t)
	sc, _ := lightingFixture(t)
	sceneDoc := ed.Docs.New("scenes/duplicate.aescene", sc)
	player, other := sc.Create(sc.Root(), "Player"), sc.Create(sc.Root(), "Other")
	for _, node := range []scene.NodeHandle{player, other} {
		sprite := scene.Add[render2d.SpriteRenderer](sc, node)
		*sprite = render2d.DefaultSpriteRenderer()
		sprite.Sprite.ID = frames[1]
	}
	sc.Flush()
	askLegacyLinkedSource(t, ed, frames[1])
	linkedSourcePrompt(t, ed).Submit([]string{"assets/linked.aeart"})
	artID, _ := ed.Pipeline.Lookup("assets/linked.aeart")
	doc := ed.Docs.AssetDocument(artID)
	model := doc.Binding.Model.(*artedit.Document)
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 1, Y: 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{G: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	pixels, _, _ := model.Capture(model.Session.Frame, art.Albedo)
	if err := askDuplicateSpriteSource(ed, sc.ID(player), frames[1]); err != nil {
		t.Fatal(err)
	}
	q := ed.TakeQuestions()
	if len(q) != 1 || q[0].Submit == nil {
		t.Fatal("duplicate did not request the source path")
	}
	q[0].Submit([]string{"assets/independent.aeart"})
	if n, ok := ed.LastNotice(); ok && n.Level == context.LevelError {
		t.Fatal(n.Text)
	}
	newID, ok := ed.Pipeline.Lookup("assets/independent.aeart")
	if !ok || newID == root || newID == artID {
		t.Fatal("independent art source not created")
	}
	copyDoc := ed.Docs.AssetDocument(newID)
	if copyDoc == nil {
		t.Fatal("independent document did not open")
	}
	copyModel := copyDoc.Binding.Model.(*artedit.Document)
	copyPixels, _, _ := copyModel.Capture(copyModel.Session.Frame, art.Albedo)
	if !bytes.Equal(copyPixels.Pix, pixels.Pix) || len(copyModel.Session.Info().Document.Frames) != 2 {
		t.Fatal("copy lost the selected dirty frame pixels or other animation frames")
	}
	playerSprite, _ := scene.Get[render2d.SpriteRenderer](sc, player)
	otherSprite, _ := scene.Get[render2d.SpriteRenderer](sc, other)
	newFrame := artsource.StableID(newID, artsource.FrameName(model.Session.Frame, art.Albedo))
	if playerSprite.Sprite.ID != newFrame || otherSprite.Sprite.ID != frames[1] || sceneDoc.History.Cursor() != 1 {
		t.Fatal("duplicate did not create one object-local assignment operation")
	}
	if !doc.Dirty() || model.History.Cursor() != 1 || doc.Binding.Model != model {
		t.Fatal("duplicate saved or replaced the original unsaved document")
	}
	if _, err := copyModel.Execute(stdctx.Background(), art.Operation{Kind: "clear"}); err != nil {
		t.Fatal(err)
	}
	unchanged, _, _ := model.Capture(model.Session.Frame, art.Albedo)
	if !bytes.Equal(unchanged.Pix, pixels.Pix) {
		t.Fatal("independent document shares mutable pixels with its original")
	}
	if err := sceneDoc.History.Undo(); err != nil || playerSprite.Sprite.ID != frames[1] {
		t.Fatal("one Undo did not restore this object's original reference", err)
	}
	if !copyDoc.Dirty() {
		t.Fatal("undoing assignment discarded work in the independent source")
	}
	if err := sceneDoc.History.Redo(); err != nil || playerSprite.Sprite.ID != newFrame {
		t.Fatal("Redo did not restore the copied source assignment", err)
	}
}

func TestDuplicateSpriteRejectsChangedDirtySource(t *testing.T) {
	ed, _, frames, _ := linkedArtEditor(t)
	sc, _ := lightingFixture(t)
	ed.Docs.New("scenes/duplicate.aescene", sc)
	player := sc.Create(sc.Root(), "Player")
	sprite := scene.Add[render2d.SpriteRenderer](sc, player)
	*sprite = render2d.DefaultSpriteRenderer()
	sprite.Sprite.ID = frames[0]
	sc.Flush()
	askLegacyLinkedSource(t, ed, frames[0])
	linkedSourcePrompt(t, ed).Submit([]string{"assets/linked.aeart"})
	artID, _ := ed.Pipeline.Lookup("assets/linked.aeart")
	model := ed.Docs.AssetDocument(artID).Binding.Model.(*artedit.Document)
	if err := askDuplicateSpriteSource(ed, sc.ID(player), frames[0]); err != nil {
		t.Fatal(err)
	}
	q := ed.TakeQuestions()[0]
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "clear"}); err != nil {
		t.Fatal(err)
	}
	q.Submit([]string{"assets/stale.aeart"})
	if n, ok := ed.LastNotice(); !ok || n.Level != context.LevelError {
		t.Fatal("duplicate accepted a stale snapshot")
	}
	if _, ok := ed.Pipeline.Lookup("assets/stale.aeart"); ok || sprite.Sprite.ID != frames[0] {
		t.Fatal("stale duplicate published source files or changed the object")
	}
}

func TestDuplicateAnimationRemapsOnlyTypedReferencesAndKeepsTiming(t *testing.T) {
	ed, _, frames, _ := linkedArtEditor(t)
	text := []byte(fmt.Sprintf("anim version=1 length=0.16 loop=loop { frames \"\" fps=12.5 { sprites=[asset(%q), asset(%q)] } event t=0.08 name=\"Footstep\" mode=presentation {} }", frames[0].String(), frames[1].String()))
	clip, err := assetops.CreateFile(ed, "assets/run.aeanim", text)
	if err != nil {
		t.Fatal(err)
	}
	controllerText := []byte(fmt.Sprintf("controller version=1 { entry=Run state \"Run\" clip=asset(%q) {} }", clip.String()))
	controller, err := assetops.CreateFile(ed, "assets/player.aectrl", controllerText)
	if err != nil {
		t.Fatal(err)
	}
	replacements := map[ids.AssetID]ids.AssetID{frames[0]: ids.NewAssetID(), frames[1]: ids.NewAssetID()}
	p := &artDuplication{ed: ed, guards: map[string][]byte{}, ids: replacements, visiting: map[ids.AssetID]bool{}, planned: map[string]bool{}, base: "assets/individual"}
	copyController, err := p.animation(controller)
	if err != nil || copyController == controller || p.ids[clip] == clip || len(p.edits) != 4 {
		t.Fatal("affected controller and clip did not get independent identities", err)
	}
	var copiedClip, copiedController []byte
	for _, edit := range p.edits {
		if edit.Path == "assets/individual-run.aeanim" {
			copiedClip = edit.After
		}
		if edit.Path == "assets/individual-player.aectrl" {
			copiedController = edit.After
		}
	}
	oldNode, _ := aetext.Parse(text, "old")
	oldClip, _ := anim.Decode(oldNode)
	node, err := aetext.Parse(copiedClip, "copy")
	if err != nil {
		t.Fatal(err)
	}
	newClip, err := anim.Decode(node)
	if err != nil || newClip.Length != oldClip.Length || newClip.Loop != oldClip.Loop || len(newClip.Events) != 1 || newClip.Events[0] != oldClip.Events[0] {
		t.Fatal("copy changed animation timing or events", err)
	}
	for _, old := range frames {
		if bytes.Contains(copiedClip, []byte(old.String())) || !bytes.Contains(copiedClip, []byte(replacements[old].String())) {
			t.Fatal("copy did not remap a legacy frame")
		}
	}
	if bytes.Contains(copiedController, []byte(clip.String())) || !bytes.Contains(copiedController, []byte(p.ids[clip].String())) {
		t.Fatal("copy controller still uses the shared animation")
	}
	if original, _ := ed.Pipeline.SourceBytes("assets/run.aeanim"); !bytes.Equal(original, text) {
		t.Fatal("duplicating rewrote the shared clip")
	}
}

func TestDuplicateSpriteRejectsPendingRecovery(t *testing.T) {
	ed, _, frames, _ := linkedArtEditor(t)
	sc, _ := lightingFixture(t)
	ed.Docs.New("scenes/recovery.aescene", sc)
	player := sc.Create(sc.Root(), "Player")
	sprite := scene.Add[render2d.SpriteRenderer](sc, player)
	*sprite = render2d.DefaultSpriteRenderer()
	sprite.Sprite.ID = frames[0]
	sc.Flush()
	askLegacyLinkedSource(t, ed, frames[0])
	linkedSourcePrompt(t, ed).Submit([]string{"assets/linked.aeart"})
	artID, ok := ed.Pipeline.Lookup("assets/linked.aeart")
	if !ok {
		t.Fatal("linked art was not created")
	}
	model := ed.Docs.AssetDocument(artID).Binding.Model.(*artedit.Document)
	model.PendingRecovery = true
	if err := askDuplicateSpriteSource(ed, sc.ID(player), frames[0]); err == nil {
		t.Fatal("pending recovery was ignored")
	}
	if len(ed.TakeQuestions()) != 0 {
		t.Fatal("opened duplicate prompt while recovery was pending")
	}
	model.PendingRecovery = false
	if err := askDuplicateSpriteSource(ed, sc.ID(player), frames[0]); err != nil {
		t.Fatal(err)
	}
	questions := ed.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatal("missing duplicate prompt")
	}
	model.PendingRecovery = true
	questions[0].Submit([]string{"assets/independent.aeart"})
	if notice, ok := ed.LastNotice(); !ok || notice.Level != context.LevelError {
		t.Fatal("duplicate accepted newly pending recovery")
	}
	if _, ok := ed.Pipeline.Lookup("assets/independent.aeart"); ok || sprite.Sprite.ID != frames[0] {
		t.Fatal("pending recovery allowed file creation or scene assignment")
	}
}

func TestSpriteInspectorOffersObjectDuplicateWithoutExtraRow(t *testing.T) {
	ed, _, frames, _ := linkedArtEditor(t)
	sc, _ := lightingFixture(t)
	ed.Docs.New("scenes/inspector-duplicate.aescene", sc)
	player := sc.Create(sc.Root(), "Player")
	sprite := scene.Add[render2d.SpriteRenderer](sc, player)
	*sprite = render2d.DefaultSpriteRenderer()
	sprite.Sprite.ID = frames[0]
	sc.Flush()
	state := &input.State{}
	draw := func() {
		ed.UI.Begin(state, mathx.Vec2{X: 450, Y: 150}, 1)
		ed.UI.Region(mathx.Rect{W: 450, H: 150})
		spriteInspector(ed, []ids.NodeID{sc.ID(player)}, func() {})
		if got, want := ed.UI.Cursor().Y, ed.UI.Theme.Padding+ed.UI.Theme.RowHeight+ed.UI.Theme.Spacing; got > want {
			t.Fatalf("sprite actions consume more than one row: %v > %v", got, want)
		}
		ed.UI.PopCursor()
		ed.UI.End()
	}
	point := mathx.Vec2{X: 375, Y: ed.UI.Theme.Padding + ed.UI.Theme.RowHeight/2}
	state.BeginFrame()
	draw()
	state.BeginFrame()
	state.ApplyMouseMove(point, mathx.Vec2{})
	state.ApplyMouseDown(point, input.MouseLeft)
	draw()
	state.BeginFrame()
	state.ApplyMouseUp(point, input.MouseLeft)
	draw()
	questions := ed.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatalf("duplicate button did not show a confirmation/path prompt: %+v", questions)
	}
	if _, ok := ed.Pipeline.Lookup("assets/linked copy.aeart"); ok {
		t.Fatal("opening the prompt wrote a source before confirmation")
	}
}
