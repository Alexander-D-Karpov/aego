//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/editor/core/workspace"
	"aego/modules/render2d"
	"aego/render/lighting"
	"aego/scene"
	"aego/scene/frame"
)

// A transparent-tinted caster is invisible to the camera but still casts its
// alpha shadow. Pixel differences therefore come from the Lit receiver/shadow,
// not from merely repainting the caster sprite over the background.
func TestArtWorkingAlphaShadowNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := newProject(t)
	drawing, err := art.New(16, 16, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	info := drawing.Info()
	first, layer := info.Frames[0].ID, info.Layers[0].ID
	e, err := drawing.Begin(drawing.Revision(), "Create two shadow frames")
	if err != nil {
		t.Fatal(err)
	}
	firstCel, err := e.EnsureCel(layer, first, art.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	for y := 4; y < 12; y++ {
		for x := 4; x < 12; x++ {
			if err := e.SetPixel(firstCel, x, y, art.Paint{Color: art.Color{R: 255, G: 255, B: 255, A: 255}}, nil, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	second, err := e.AddFrame(first, 100)
	if err != nil {
		t.Fatal(err)
	}
	secondCel, err := e.EnsureCel(layer, second, art.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if err := e.SetPixel(secondCel, x, y, art.Paint{Color: art.Color{R: 255, G: 255, B: 255, A: 255}}, nil, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	change, err := e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if err := change.Apply(); err != nil {
		t.Fatal(err)
	}
	data, err := drawing.Encode()
	drawing.Close()
	if err != nil {
		t.Fatal(err)
	}
	const artPath = "assets/shadow.aeart"
	if err := os.WriteFile(filepath.Join(p.Root, artPath), data, 0644); err != nil {
		t.Fatal(err)
	}
	imp := artsource.New()
	meta := asset.NewSourceMeta(imp.ID(), imp.Version(), imp.Defaults())
	root := ids.NewAssetID()
	meta.ID = root
	if err := os.WriteFile(filepath.Join(p.Root, artPath+".meta"), asset.MarshalSourceMeta(meta), 0644); err != nil {
		t.Fatal(err)
	}
	const scenePath = "scenes/art-shadow.aescene"
	writeScene(t, p, scenePath, func(sc *scene.Scene) {
		camera := sc.Create(sc.Root(), "Camera")
		*scene.Add[render2d.Camera2D](sc, camera) = render2d.DefaultCamera2D()
		receiver := sc.Create(sc.Root(), "Lit receiver")
		sr := scene.Add[render2d.SpriteRenderer](sc, receiver)
		*sr = render2d.DefaultSpriteRenderer()
		sr.Sprite = asset.NewRef[asset.Texture](asset.BuiltinWhite)
		sr.Material = asset.NewRef[asset.Material](asset.BuiltinMaterialLit)
		sr.Size = mathx.V2(14, 14)
		sr.Order = -1
		caster := sc.Create(sc.Root(), "Invisible alpha caster")
		sp := scene.Add[render2d.SpriteRenderer](sc, caster)
		*sp = render2d.DefaultSpriteRenderer()
		sp.Sprite.ID, sp.Size, sp.Tint = root, mathx.V2(2, 2), mathx.Color{R: 1, G: 1, B: 1, A: 0}
		oc := scene.Add[render2d.LightOccluder2D](sc, caster)
		*oc = render2d.DefaultLightOccluder2D()
		oc.Source = render2d.OccluderSpriteAlpha
		lamp := sc.Create(sc.Root(), "Point light")
		light := scene.Add[render2d.Light2D](sc, lamp)
		*light = render2d.DefaultLight2D()
		light.Range, light.Intensity, light.Shadows = 9, 2, true
		pose := sc.Local(lamp)
		pose.Position.X = -3
		sc.SetLocal(lamp, pose)
		env := scene.Add[render2d.LightingEnvironment2D](sc, sc.Create(sc.Root(), "Environment"))
		*env = render2d.DefaultLightingEnvironment2D()
		env.AmbientIntensity, env.MaxShadowedLights = .12, 4
	})
	s, err := Open(p, Config{Width: 1000, Height: 760, Hidden: true, NoWatch: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Step() {
		t.Fatal("editor startup")
	}
	ed, ws := s.App.Context(), s.App.Workspace()
	sceneDoc, err := ed.Docs.Open(scenePath)
	if err != nil {
		t.Fatal(err)
	}
	sceneDoc.View.Camera.Position = mathx.Vec2{}
	panel := s.App.SceneView()
	panel.SetZoom(40)
	panel.SetLightingView(true, lighting.DebugShadows)
	ed.Settings.Grid = false
	ws.Show(workspace.Scene)
	ws.Focus(workspace.Scene)
	if ws.Tree.Maximized() != workspace.Scene && !ws.ToggleMaximize(workspace.Scene) {
		t.Fatal("cannot show Scene View")
	}
	for range 5 {
		if !s.Step() {
			t.Fatal("scene setup")
		}
	}
	artDoc, err := ed.OpenAsset(root)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := artDoc.Binding.Model.(*artedit.Document)
	if !ok {
		t.Fatal("missing art document")
	}
	firstID := artsource.StableID(root, artsource.FrameName(first, art.Albedo))
	secondID := artsource.StableID(root, artsource.FrameName(second, art.Albedo))
	wait := func(previous uint64) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for artDoc.WorkingPreview.Applied == previous && artDoc.WorkingPreview.Error == "" && time.Now().Before(deadline) {
			if !s.Step() {
				t.Fatal("preview stopped")
			}
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		if artDoc.WorkingPreview.Applied == previous || artDoc.WorkingPreview.Error != "" || artDoc.WorkingPreview.Pending {
			t.Fatalf("preview: %+v", artDoc.WorkingPreview)
		}
	}
	read := func(name string) ([]byte, []mathx.Vec2) {
		t.Helper()
		ws.Focus(workspace.Scene)
		for range 4 {
			if !s.Step() {
				t.Fatal("scene stopped")
			}
		}
		raw, ok := panel.LightingWorld().ServiceAny(frame.ServiceID)
		if !ok {
			t.Fatal("Scene View runner missing")
		}
		runner := raw.(*frame.Runner)
		if len(runner.Views()) != 1 {
			t.Fatal("expected one Scene View")
		}
		input := runner.Bucket(0).Lighting
		if len(input.Lights) != 1 || input.Disabled || input.Debug != lighting.DebugShadows {
			t.Fatalf("scene lighting not active: %+v", input)
		}
		target, w, h := runner.Target()
		pixels := make([]byte, w*h*4)
		if target.IsZero() || ed.Renderer.Device().ReadPixels(target, 0, 0, w, h, pixels) != nil {
			t.Fatal("cannot read rendered Scene View")
		}
		if dir := os.Getenv("AEGO_ART_SHADOW_SHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			im := &image.RGBA{Pix: pixels, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
			if err := png.Encode(f, im); err != nil {
				f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
		}
		return pixels, slices.Clone(input.Points)
	}
	beforeShape, ok := ed.Loader.SpriteShadow(root)
	if !ok || len(beforeShape.Parts) == 0 {
		t.Fatal("initial first-frame shadow missing")
	}
	secondShape, ok := ed.Loader.SpriteShadow(secondID)
	if !ok || len(secondShape.Parts) == 0 {
		t.Fatal("second-frame alpha must exist")
	}
	before, geometry := read("before")
	if len(geometry) == 0 {
		t.Fatal("renderer extracted no initial occluder")
	}
	previous := artDoc.WorkingPreview.Applied
	if _, err := model.Execute(context.Background(), art.Operation{Kind: "clear"}); err != nil {
		t.Fatal(err)
	}
	wait(previous)
	rootShape, rootOK := ed.Loader.SpriteShadow(root)
	firstShape, firstOK := ed.Loader.SpriteShadow(firstID)
	secondShape, secondOK := ed.Loader.SpriteShadow(secondID)
	if !rootOK || !firstOK || !secondOK || len(rootShape.Parts) != 0 || len(firstShape.Parts) != 0 || len(secondShape.Parts) == 0 {
		t.Fatalf("wrong working alpha: root=%+v first=%+v second=%+v", rootShape, firstShape, secondShape)
	}
	after, afterGeometry := read("after")
	if len(afterGeometry) != 0 || bytes.Equal(before, after) {
		t.Fatal("actual Scene View shadow did not change when only first-frame alpha was cleared")
	}
	previous = artDoc.WorkingPreview.Applied
	if err := ed.UndoAsset(artDoc); err != nil {
		t.Fatal(err)
	}
	wait(previous)
	restored, restoredGeometry := read("undo")
	if !slices.Equal(geometry, restoredGeometry) || !bytes.Equal(before, restored) {
		t.Fatal("Undo did not restore rendered alpha shadow")
	}
	t.Logf("native shadow: initial=%d vertices, cleared=%d, undo=%d; GPU pixels changed and restored", len(geometry), len(afterGeometry), len(restoredGeometry))
}
