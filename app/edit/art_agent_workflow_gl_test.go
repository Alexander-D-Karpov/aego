//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"aego/anim"
	"aego/app/agent"
	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/editor/core/play"
	"aego/editor/core/workspace"
	"aego/modules/animation"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/platform/sdl"
	"aego/scene"
	"aego/scene/sceneio"
)

// Local fake provider, actual approval/mutation bridge, actual SDL input and
// actual Game View GPU pixels. This does not claim an authorized remote model.
func TestArtAgentNativeAssignGameViewUndo(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("AEGO_LANG", "en")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	p := newProject(t)
	drawing, err := art.New(16, 16, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	first := drawing.Info().Frames[0].ID
	source, err := drawing.Encode()
	drawing.Close()
	if err != nil {
		t.Fatal(err)
	}
	const artPath, scenePath = "assets/agent-animated.aeart", "scenes/agent-animated.aescene"
	sourceID := ids.NewAssetID()
	imp := artsource.New()
	meta := asset.NewSourceMeta(imp.ID(), imp.Version(), imp.Defaults())
	meta.ID = sourceID
	for rel, data := range map[string][]byte{artPath: source, artPath + ".meta": asset.MarshalSourceMeta(meta)} {
		if err = os.WriteFile(filepath.Join(p.Root, rel), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeScene(t, p, scenePath, func(sc *scene.Scene) {
		camera := sc.Create(sc.Root(), "Camera")
		*scene.Add[render2d.Camera2D](sc, camera) = render2d.DefaultCamera2D()
		hero := sc.Create(sc.Root(), "Hero")
		sprite := scene.Add[render2d.SpriteRenderer](sc, hero)
		*sprite = render2d.DefaultSpriteRenderer()
		sprite.Size = mathx.V2(4, 4)
		*scene.Add[animation.AnimationPlayer](sc, hero) = animation.DefaultPlayer()
	})
	sceneDisk, err := os.ReadFile(filepath.Join(p.Root, scenePath))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop(), AgentBackend: &backend.Fake{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Engine.SetLockstep(true)
	step := func() {
		t.Helper()
		if !s.Step() {
			t.Fatal("editor stopped during agent/native authoring workflow")
		}
	}
	step()
	ed, ws := s.App.Context(), s.App.Workspace()
	sceneDoc, err := ed.Docs.Open(scenePath)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ed.OpenAsset(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok {
		t.Fatal("Art source did not acquire the common document model")
	}
	bridge, err := s.agentBridge()
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		out *agent.Output
		err error
	}
	call := func(name string, args agent.Args, approve func(*agentbridge.Proposal)) (*agent.Output, int) {
		t.Helper()
		done := make(chan result, 1)
		go func() {
			out, err := bridge.Run(name, args)
			done <- result{out, err}
		}()
		proposalID := 0
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			step()
			for _, pending := range bridge.Pending() {
				if approve == nil {
					t.Fatal("read command unexpectedly proposed a mutation", name)
				}
				approve(pending)
				proposalID = pending.ID
				if !bridge.Decide(pending.ID, true) {
					t.Fatal("could not accept the actual proposal", name)
				}
			}
			select {
			case r := <-done:
				if r.err != nil || r.out == nil {
					t.Fatalf("%s: %v", name, r.err)
				}
				if approve != nil && proposalID == 0 {
					t.Fatal("write bypassed proposal approval", name)
				}
				return r.out, proposalID
			default:
			}
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		t.Fatal("agent/native command timed out", name)
		return nil, 0
	}
	readRevision := func() string {
		t.Helper()
		out, _ := call("art.read", agent.Args{"file": artPath}, nil)
		value, ok := out.Value.(map[string]any)
		if !ok || value["documentID"] != model.Session.Document.Info().ID {
			t.Fatal("agent resolved another Art document", out.Value)
		}
		revision, ok := value["revision"].(string)
		if !ok || revision != strconv.FormatUint(model.Session.Document.Revision(), 10) {
			t.Fatal("agent received a stale document revision", out.Value)
		}
		return revision
	}
	colors := []art.Color{
		{R: 240, G: 32, B: 32, A: 255}, {R: 32, G: 220, B: 32, A: 255},
		{R: 32, G: 32, B: 240, A: 255}, {R: 240, G: 220, B: 32, A: 255},
		{R: 220, G: 32, B: 220, A: 255}, {R: 32, G: 220, B: 220, A: 255},
		{R: 240, G: 128, B: 32, A: 255}, {R: 128, G: 96, B: 160, A: 255},
	}
	layer, last := model.Session.Layer, first
	operations := []art.Operation{{Kind: "duration", Frame: first, DurationMS: 100}}
	for i, color := range colors {
		if i != 0 {
			name := fmt.Sprintf("frame%d", i)
			operations = append(operations, art.Operation{Kind: "add-frame", Frame: last, DurationMS: 100, As: name})
			last = art.ID("$" + name)
		}
		operations = append(operations, art.Operation{Kind: "rectangle", Layer: layer, Frame: last, Filled: true,
			Points: []image.Point{{0, 0}, {15, 15}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: color}}})
	}
	_, createProposal := call("art.apply", agent.Args{"file": artPath, "revision": readRevision(), "label": "Agent draws eight frames", "operations": operations}, func(p *agentbridge.Proposal) {
		if p.Preview.Art == nil || len(model.Session.Document.Info().Frames) != 1 || model.History.Cursor() != 0 {
			t.Fatal("Art batch published before its approval")
		}
	})
	if len(model.Session.Document.Info().Frames) != 8 || model.History.Cursor() != 1 || !bridge.CanUndo(createProposal) {
		t.Fatal("approved eight-frame drawing was not one shared History step")
	}
	canvasID := "render2d.art/" + sourceID.String()
	maximize := func(id string) mathx.Rect {
		t.Helper()
		ws.Tree.Restore()
		ws.Show(id)
		ws.Focus(id)
		if !ws.ToggleMaximize(id) {
			t.Fatal("cannot maximize native panel", id)
		}
		for range 3 {
			step()
		}
		r, ok := ws.Tree.PanelRect(id)
		if !ok {
			t.Fatal("native panel has no visible rectangle", id)
		}
		return r
	}
	writeShot := func(name string, data []byte) {
		t.Helper()
		if dir := os.Getenv("AEGO_ART_AGENT_SHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name+".png"), data, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	beforePointer, _, err := model.Capture(first, art.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	canvas := maximize(canvasID)
	for kind := range 3 {
		if !sdl.TestPointerEvent(canvas.X+canvas.W*.4, canvas.Y+canvas.H*.4, kind) {
			t.Fatal("actual SDL user gesture failed")
		}
		step()
	}
	afterPointer, _, err := model.Capture(first, art.Albedo)
	if err != nil || bytes.Equal(beforePointer.Pix, afterPointer.Pix) || model.History.Cursor() != 2 || !doc.Dirty() {
		t.Fatal("actual pointer correction did not change the agent's dirty first frame", err)
	}
	var uiShot *image.RGBA
	var shotErr error
	s.Engine.CaptureFrame(func(pixels *image.RGBA, err error) { uiShot, shotErr = pixels, err })
	step()
	if shotErr != nil || uiShot == nil {
		t.Fatal("native UI screenshot failed", shotErr)
	}
	var uiBytes bytes.Buffer
	if err = png.Encode(&uiBytes, uiShot); err != nil {
		t.Fatal(err)
	}
	writeShot("01-eight-frames-user-pointer", uiBytes.Bytes())
	readRevision()
	captured, _ := call("art.capture", agent.Args{"file": artPath}, nil)
	if len(captured.Images) != 1 || captured.Images[0].Mime != "image/png" {
		t.Fatal("agent did not receive a real current PNG")
	}
	readPixels, err := png.Decode(bytes.NewReader(captured.Images[0].Data))
	if err != nil || readPixels.Bounds() != afterPointer.Bounds() {
		t.Fatal("agent PNG dimensions", err)
	}
	for y := range 16 {
		for x := range 16 {
			if color.NRGBAModel.Convert(readPixels.At(x, y)) != afterPointer.NRGBAAt(x, y) {
				t.Fatal("agent captured stale disk/agent pixels after the actual user stroke", x, y)
			}
		}
	}
	writeShot("02-agent-reads-dirty-pixels", captured.Images[0].Data)
	clipID := artsource.StableID(sourceID, artsource.ClipName(""))
	editScene := sceneDoc.Scene()
	hero := editScene.Find(editScene.Root(), "Hero")
	sprite, _ := scene.Get[render2d.SpriteRenderer](editScene, hero)
	oldSprite := sprite.Sprite.ID
	beforeAssignment := sceneio.Save(editScene)
	transaction := fmt.Sprintf(`tx version=1 { label="Assign authored animation" op "set" { node="Hero" path="SpriteRenderer.sprite" value=asset(%q) } op "set" { node="Hero" path="AnimationPlayer.clip" value=asset(%q) } }`, sourceID.String(), clipID.String())
	_, assignProposal := call("scene.apply", agent.Args{"file": scenePath, "tx": transaction}, func(p *agentbridge.Proposal) {
		if !bytes.Equal(sceneio.Save(editScene), beforeAssignment) || sprite.Sprite.ID != oldSprite {
			t.Fatal("assignment changed the scene before approval")
		}
	})
	player, _ := scene.Get[animation.AnimationPlayer](editScene, hero)
	if sprite.Sprite.ID != sourceID || player.Clip.ID != clipID || !bridge.CanUndo(assignProposal) {
		t.Fatal("approved typed assignment did not use SpriteRenderer/AnimationPlayer and common scene History")
	}
	assignedScene := sceneio.Save(editScene)
	waitPreview := func() {
		t.Helper()
		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			step()
			if doc.WorkingPreview.Error != "" {
				t.Fatal("dirty art preparation failed", doc.WorkingPreview.Error)
			}
			if ed.WorkingSourceCurrent(doc, model.PreviewRevision()) && !doc.WorkingPreview.Pending {
				return
			}
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		t.Fatal("dirty art preview did not publish current revision", doc.WorkingPreview, model.PreviewRevision())
	}
	waitPreview()
	clipBytes, ok := ed.Loader.Anim(clipID)
	if !ok {
		t.Fatal("assigned dirty runtime animation is unavailable")
	}
	clip, err := anim.Unmarshal(clipBytes)
	if err != nil || len(clip.Tracks) != 1 || len(clip.Tracks[0].Keys) != 8 {
		t.Fatal("current dirty drawing did not prepare an eight-frame runtime clip", err)
	}
	maximize(workspace.Game)
	key := func(code input.Scancode, ctrl bool) {
		t.Helper()
		if !sdl.TestEditKey(code, true, ctrl, false) {
			t.Fatal("native key press failed")
		}
		step()
		if !sdl.TestEditKey(code, false, false, false) {
			t.Fatal("native key release failed")
		}
		step()
	}
	key(input.KeyP, true) // Actual editor Play shortcut.
	if s.App.Play().State() != play.Playing {
		t.Fatal("actual Play shortcut did not start the assigned scene")
	}
	defer s.App.Play().Stop()
	frameIDs := make(map[ids.AssetID]int)
	for i, frame := range model.Session.Document.Info().Frames {
		frameIDs[artsource.StableID(sourceID, artsource.FrameName(frame.ID, art.Albedo))] = i
	}
	gameFrame := func() ids.AssetID {
		world := s.App.Play().Scene()
		if world == nil {
			t.Fatal("Game View has no runtime scene")
		}
		node := world.Find(world.Root(), "Hero")
		sprite, ok := scene.Get[render2d.SpriteRenderer](world, node)
		if !ok {
			t.Fatal("runtime Hero lost SpriteRenderer")
		}
		return sprite.Sprite.ID
	}
	captureGame := func(name string) ([]byte, uint64) {
		t.Helper()
		// Showing an agent proposal opens its dock. Return to the same actual
		// Game View viewport before comparing GPU pixels across that UI action.
		maximize(workspace.Game)
		data, tick, path, err := s.App.CaptureAgentView("game")
		if err != nil || path != scenePath {
			t.Fatal("actual Game View capture failed", path, err)
		}
		writeShot(name, data)
		return data, tick
	}
	seen := make(map[int]bool)
	for n := 0; n < 120 && len(seen) < 8; n++ {
		step()
		index, found := frameIDs[gameFrame()]
		if !found || seen[index] {
			continue
		}
		s.App.Play().Pause()
		data, _ := captureGame(fmt.Sprintf("game-frame-%02d", index+1))
		pixels, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		visible := 0
		for y := pixels.Bounds().Min.Y; y < pixels.Bounds().Max.Y; y++ {
			for x := pixels.Bounds().Min.X; x < pixels.Bounds().Max.X; x++ {
				c := color.NRGBAModel.Convert(pixels.At(x, y)).(color.NRGBA)
				near := func(a, b uint8) bool {
					delta := int(a) - int(b)
					return delta >= -12 && delta <= 12
				}
				if near(c.R, colors[index].R) && near(c.G, colors[index].G) && near(c.B, colors[index].B) {
					visible++
				}
			}
		}
		if visible < 100 {
			t.Fatalf("Game View did not display authored frame %d: %d matching pixels", index+1, visible)
		}
		seen[index] = true
		s.App.Play().Pause()
	}
	if len(seen) != 8 {
		t.Fatal("real runtime did not play all eight agent-authored frames", seen)
	}
	firstID := artsource.StableID(sourceID, artsource.FrameName(first, art.Albedo))
	for n := 0; n < 60 && gameFrame() != firstID; n++ {
		step()
	}
	if gameFrame() != firstID {
		t.Fatal("animation did not loop back to the user's corrected frame")
	}
	s.App.Play().Pause()
	beforeEdit, tick := captureGame("03-game-user-corrected")
	beforePixels, _, err := model.Capture(first, art.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	_, patchProposal := call("art.apply", agent.Args{"file": artPath, "revision": readRevision(), "label": "Accepted agent correction after preview", "operations": []art.Operation{{Kind: "rectangle", Layer: layer, Frame: first, Filled: true, Points: []image.Point{{0, 0}, {15, 15}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{G: 255, A: 255}}}}}}, func(p *agentbridge.Proposal) {
		if model.History.Cursor() != 2 {
			t.Fatal("agent correction published before approval")
		}
	})
	waitPreview()
	changed, changedTick := captureGame("04-game-agent-correction")
	if model.History.Cursor() != 3 || bytes.Equal(beforeEdit, changed) || changedTick != tick || !bridge.CanUndo(patchProposal) {
		t.Fatal("approved operation did not change actual paused Game View pixels as one History step")
	}
	if err := bridge.Undo(patchProposal); err != nil {
		t.Fatal("agent Undo did not use shared Art history", err)
	}
	waitPreview()
	restored, restoredTick := captureGame("05-game-common-undo")
	restoredPixels, _, err := model.Capture(first, art.Albedo)
	if err != nil || !bytes.Equal(beforePixels.Pix, restoredPixels.Pix) || !bytes.Equal(beforeEdit, restored) || restoredTick != tick || model.History.Cursor() != 2 {
		t.Fatalf("common Undo restoration: pixels=%v game=%v tick=%d/%d history=%d error=%v", bytes.Equal(beforePixels.Pix, restoredPixels.Pix), bytes.Equal(beforeEdit, restored), restoredTick, tick, model.History.Cursor(), err)
	}
	if !bytes.Equal(sceneio.Save(editScene), assignedScene) || len(model.Session.Document.Info().Frames) != 8 {
		t.Fatal("art correction/Undo changed the assigned scene or frame topology")
	}
	for rel, want := range map[string][]byte{artPath: source, scenePath: sceneDisk} {
		got, err := os.ReadFile(filepath.Join(p.Root, rel))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("preview/agent workflow silently saved a dirty source", rel, err)
		}
	}
	t.Log("local fake provider: approved eight-frame Art batch -> actual SDL user correction -> current dirty PNG -> approved SpriteRenderer/AnimationPlayer assignment -> all eight frames in real Game View -> accepted pixel correction -> common Undo restored user pixels and GPU frame; no implicit Save")
}
