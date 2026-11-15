//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/editor/core/command"
	"aego/editor/core/workspace"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/platform/sdl"
	"aego/scene"
)

func TestPlatformerEditSourceArtNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("AEGO_LANG", "en")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()}
	frames := map[string][]byte{}
	var expectedPlayerFrame ids.AssetID
	var gamePlayerBounds image.Rectangle
	var gameTick uint64
	click := func(t *testing.T, s *Session, x, y float32) {
		t.Helper()
		for i := range 3 {
			if !sdl.TestPointerEvent(x, y, i) || !s.Step() {
				t.Fatal("native pointer event failed")
			}
		}
	}
	capture := func(t *testing.T, s *Session, name string) *image.RGBA {
		t.Helper()
		dir := os.Getenv("AEGO_PLATFORMER_ART_SHOTS")
		var pixels *image.RGBA
		var err error
		s.Engine.CaptureFrame(func(img *image.RGBA, e error) { pixels, err = img, e })
		if !s.Step() || err != nil || pixels == nil {
			t.Fatal("native screenshot failed", err)
		}
		if dir == "" {
			return pixels
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		out, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(out, pixels)
		if closeErr := out.Close(); err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
		return pixels
	}
	platformerSourceArtWorkflow(t, cfg, func(t *testing.T, s *Session, frameID ids.AssetID) {
		expectedPlayerFrame = frameID
		ed, ws := s.App.Context(), s.App.Workspace()
		pngBefore, err := s.Pipeline.SourceBytes("assets/surfaces/runner.png")
		if err != nil {
			t.Fatal(err)
		}
		metaBefore, err := s.Pipeline.SourceBytes("assets/surfaces/runner.png.meta")
		if err != nil || len(metaBefore) == 0 {
			t.Fatal("runner metadata missing", err)
		}
		ws.Focus(workspace.Inspector)
		if ws.Tree.Maximized() != workspace.Inspector && !ws.ToggleMaximize(workspace.Inspector) {
			t.Fatal("could not maximize the Player Inspector")
		}
		ed.Collapsed["engine.SpriteRenderer"] = false
		s.App.Inspector().ScrollTo("engine.SpriteRenderer")
		for range 3 {
			if !s.Step() {
				t.Fatal("native Inspector did not render")
			}
		}
		capture(t, s, "01-player-inspector")
		ui := ed.UI
		items := s.App.UIList().TestDrawnItems(mathx.Rect{W: ui.Size().X, H: ui.Size().Y}, ui.Font(), "Edit Source Art", 0, 0)
		if len(items) != 1 {
			t.Fatalf("expected one visible Edit Source Art action, got %+v", items)
		}
		button := items[0].Bounds
		click(t, s, button.X+button.W/2, button.Y+button.H/2)
		capture(t, s, "02-png-draft-open")
		if ed.UI.ModalActive() {
			t.Fatal("opening the PNG unexpectedly required a conversion prompt")
		}
		assertPlayerLinkPending(t, s, pngBefore, metaBefore)
		root, ok := s.Pipeline.Lookup("assets/surfaces/runner.png")
		if !ok {
			t.Fatal("PNG source identity disappeared")
		}
		ws.Focus("render2d.art/" + root.String())
		if err := ed.Commands.Run(command.FileSave); err != nil || !s.Step() {
			t.Fatal("first Save did not request a source path", err)
		}
		if !s.Step() {
			t.Fatal("first-save prompt stopped")
		}
		if !ed.UI.ModalActive() || ed.UI.Focus() == 0 {
			t.Fatal("first Save did not open a focused source path prompt")
		}
		assertPlayerLinkPending(t, s, pngBefore, metaBefore)
		capture(t, s, "03-linked-source-path")
		for _, down := range []bool{true, false} {
			if !sdl.TestEditKey(input.KeyEnter, down, false, false) || !s.Step() {
				t.Fatal("native linked source path submission failed")
			}
		}
		capture(t, s, "03b-linked-source-open")
	}, func(t *testing.T, s *Session, panelID string, model *artedit.Document) {
		ws := s.App.Workspace()
		ws.Focus(panelID)
		if ws.Tree.Maximized() != panelID && !ws.ToggleMaximize(panelID) {
			t.Fatal("could not maximize the real Player art canvas")
		}
		for range 3 {
			if !s.Step() {
				t.Fatal("native art document did not render")
			}
		}
		capture(t, s, "04-player-eight-frame-canvas")
		click(t, s, 500, 400)
		if model.History.Cursor() != 1 {
			t.Fatalf("one SDL stroke produced %d undo operations", model.History.Cursor())
		}
		capture(t, s, "05-player-painted")
	}, func(t *testing.T, s *Session, panelID, stage string) {
		ws := s.App.Workspace()
		view, target := "scene", workspace.Scene
		if strings.HasPrefix(stage, "game-") {
			view, target = "game", workspace.Game
		}
		if current := ws.Tree.Maximized(); current != "" && current != target {
			ws.ToggleMaximize(current)
		}
		ws.Show(target)
		ws.Focus(target)
		if ws.Tree.Maximized() != target && !ws.ToggleMaximize(target) {
			t.Fatal("could not maximize the real platformer view", view)
		}
		for range 4 {
			if !s.Step() {
				t.Fatal("native platformer view stopped", view)
			}
		}
		if stage == "scene-before" {
			ed := s.App.Context()
			sc := ed.Doc().Scene()
			player := sc.Find(sc.Root(), "Player")
			selected, ok := ed.Selection.Active()
			if !ok || selected.Node != sc.ID(player) {
				t.Fatal("opening source art lost the selected canonical Player")
			}
			// Frame Selection is the ordinary editor action. The previous test
			// left the camera elsewhere and only photographed shared consumers.
			if err := ed.Commands.Run(command.SceneFrame); err != nil {
				t.Fatal("Frame Selection could not focus the actual Player", err)
			}
			for range 3 {
				if !s.Step() {
					t.Fatal("native selected-Player Scene View stopped")
				}
			}
		}
		data, tick, scenePath, err := s.App.CaptureAgentView(view)
		if err != nil || scenePath != "scenes/main.aescene" {
			t.Fatalf("%s capture: scene=%q err=%v", stage, scenePath, err)
		}
		frames[stage] = data
		if dir := os.Getenv("AEGO_PLATFORMER_ART_SHOTS"); dir != "" {
			if err := os.WriteFile(filepath.Join(dir, stage+".png"), data, 0644); err != nil {
				t.Fatal(err)
			}
		}
		if view == "game" {
			pc := s.App.Play()
			sc := pc.Scene()
			player := sc.Find(sc.Root(), "Player")
			sprite, ok := scene.Get[render2d.SpriteRenderer](sc, player)
			if !ok || sprite.Sprite.ID != expectedPlayerFrame || pc.Ticks() != tick || len(pc.Views()) != 1 {
				t.Fatal("Game View did not capture the actual paused Player frame", stage, tick)
			}
			pixels, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			v, transform := pc.Views()[0], sc.RenderWorld(player)
			lo, hi := mathx.V2(float32(math.Inf(1)), float32(math.Inf(1))), mathx.V2(float32(math.Inf(-1)), float32(math.Inf(-1)))
			for _, corner := range []mathx.Vec2{{}, {X: 1}, {X: 1, Y: 1}, {Y: 1}} {
				point := v.WorldToScreen(transform.Apply(corner.Sub(sprite.Origin).Mul(sprite.Size)))
				lo.X, lo.Y = min(lo.X, point.X), min(lo.Y, point.Y)
				hi.X, hi.Y = max(hi.X, point.X), max(hi.Y, point.Y)
			}
			bounds := image.Rect(int(math.Floor(float64(lo.X)))-2, int(math.Floor(float64(lo.Y)))-2, int(math.Ceil(float64(hi.X)))+2, int(math.Ceil(float64(hi.Y)))+2)
			if bounds.Empty() || !bounds.In(pixels.Bounds()) || bounds.Dx() < 16 || bounds.Dy() < 16 {
				t.Fatalf("actual Player is not fully visible in Game View: %v within %v", bounds, pixels.Bounds())
			}
			if stage == "game-painted" {
				gamePlayerBounds, gameTick = bounds, tick
			} else if gamePlayerBounds != bounds || gameTick != tick {
				t.Fatal("paused Player transform/tick changed during art Undo/Redo", stage)
			}
			nativePixels := capture(t, s, "native-"+stage)
			ui := s.App.Context().UI
			var displayed mathx.Rect
			for _, item := range s.App.UIList().TestDrawnItems(mathx.Rect{W: ui.Size().X, H: ui.Size().Y}, ui.Font(), "", pixels.Bounds().Dx(), pixels.Bounds().Dy()) {
				if item.Bounds.W == float32(pixels.Bounds().Dx()) && item.Bounds.H == float32(pixels.Bounds().Dy()) {
					displayed = item.Bounds
					break
				}
			}
			if displayed.W <= 0 || displayed.H <= 0 {
				t.Fatal("actual Game View texture is absent from the native UI")
			}
			// Compare every pixel in the projected Player rectangle against
			// the actual presented native frame. Offscreen capture must have
			// the same upright orientation, not merely an equal frame average.
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					r, g, b, _ := pixels.At(x, y).RGBA()
					nr, ng, nb, _ := nativePixels.At(x+int(displayed.X), y+int(displayed.Y)).RGBA()
					if r != nr || g != ng || b != nb {
						t.Fatalf("agent PNG differs from actual upright Player pixel at %d,%d in %s", x, y, stage)
					}
				}
			}
			if dir := os.Getenv("AEGO_PLATFORMER_ART_SHOTS"); dir != "" {
				receipt := struct {
					PlayerNode, AssetRef string
					Tick                 uint64
					PixelBounds          image.Rectangle
					NativeViewport       mathx.Rect
					FlipX                bool
				}{sc.ID(player).String(), sprite.Sprite.ID.String(), tick, bounds, displayed, sprite.FlipX}
				encoded, err := json.MarshalIndent(receipt, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, stage+".json"), append(encoded, '\n'), 0644); err != nil {
					t.Fatal(err)
				}
			}
		}
		ws.ToggleMaximize(target)
		ws.Focus(panelID)
	})
	if bytes.Equal(frames["scene-before"], frames["scene-painted"]) || !bytes.Equal(frames["scene-before"], frames["scene-undo"]) || !bytes.Equal(frames["scene-painted"], frames["scene-redo"]) {
		t.Fatal("native Player GPU pixels did not change and restore through paint/Undo/Redo")
	}
	if bytes.Equal(frames["scene-painted"], frames["scene-cleared-alpha"]) {
		t.Fatal("native Player GPU view did not change when frame alpha was cleared")
	}
	painted, err := png.Decode(bytes.NewReader(frames["game-painted"]))
	if err != nil {
		t.Fatal(err)
	}
	undone, err := png.Decode(bytes.NewReader(frames["game-undo"]))
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	for y := gamePlayerBounds.Min.Y; y < gamePlayerBounds.Max.Y; y++ {
		for x := gamePlayerBounds.Min.X; x < gamePlayerBounds.Max.X; x++ {
			if painted.At(x, y) != undone.At(x, y) {
				changed++
			}
		}
	}
	if changed == 0 || !bytes.Equal(frames["game-painted"], frames["game-redo"]) {
		t.Fatalf("actual Player Game View pixels did not change and restore: changed within Player=%d", changed)
	}
	t.Logf("actual canonical Player visible in Game View at tick %d: %d changed pixels in bounds %v; exact Redo restored GPU output", gameTick, changed, gamePlayerBounds)
}
