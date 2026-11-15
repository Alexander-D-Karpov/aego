//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"aego/art"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/editor/core/document"
	"aego/editor/core/workspace"
	"aego/modules/render2d"
	"aego/platform/sdl"
	"aego/scene"
	"aego/ui/dock"
)

func TestArtWorkspaceContinuityNative(t *testing.T) {
	for _, profile := range []struct {
		name    string
		w, h    int
		density float32
	}{{"720p", 1280, 720, 1}, {"1080p", 1920, 1080, 1}, {"narrow", 720, 900, 1}, {"hidpi", 1920, 1080, 2}} {
		t.Run(profile.name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			p := newProject(t)
			d, err := art.New(16, 16, art.RGBA, art.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			e, err := d.Begin(d.Revision(), "Eight frames")
			if err != nil {
				t.Fatal(err)
			}
			last := d.Info().Frames[0].ID
			for range 7 {
				last, err = e.AddFrame(last, 80)
				if err != nil {
					t.Fatal(err)
				}
			}
			change, err := e.Prepare()
			if err != nil || change.Apply() != nil {
				t.Fatal("prepare animation", err)
			}
			encoded, err := d.Encode()
			d.Close()
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"first", "second"} {
				if err := os.WriteFile(filepath.Join(p.Root, "assets", name+".aeart"), encoded, 0644); err != nil {
					t.Fatal(err)
				}
			}
			writeScene(t, p, "scenes/workspaces.aescene", func(sc *scene.Scene) {
				camera := sc.Create(sc.Root(), "Camera")
				*scene.Add[render2d.Camera2D](sc, camera) = render2d.DefaultCamera2D()
				sc.Create(sc.Root(), "Selected")
			})
			cfg := Config{Hidden: true, NoWatch: true, Width: profile.w, Height: profile.h, Logger: log.Nop()}
			s, err := Open(p, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			s.App.SetDensity(profile.density)
			step := func() {
				t.Helper()
				if !s.Step() {
					t.Fatal("editor stopped while switching workspaces")
				}
			}
			step()
			ed, ws := s.App.Context(), s.App.Workspace()
			sceneDoc, err := ed.Docs.Open("scenes/workspaces.aescene")
			if err != nil {
				t.Fatal(err)
			}
			sc := sceneDoc.Scene()
			selected := []ids.NodeID{sc.ID(sc.Find(sc.Root(), "Selected"))}
			ed.Select(selected...)
			type state struct {
				doc      *document.AssetDocument
				model    *artedit.Document
				panel    workspace.Panel
				panelID  string
				frame    art.ID
				pixels   []byte
				revision uint64
			}
			var opened []state
			for _, name := range []string{"first", "second"} {
				doc, err := ed.OpenAssetPath("assets/" + name + ".aeart")
				if err != nil {
					t.Fatal(err)
				}
				model := doc.Binding.Model.(*artedit.Document)
				frame := model.Session.Info().Document.Frames[5].ID
				if err := model.Session.Select("", frame, art.Albedo); err != nil {
					t.Fatal(err)
				}
				panelID := "render2d.art/" + doc.Asset.String()
				ws.Focus(panelID)
				if !ws.ToggleMaximize(panelID) {
					t.Fatal("cannot maximize the art document in a narrow layout")
				}
				for range 3 {
					step()
				}
				r, ok := ws.Tree.PanelRect(panelID)
				if !ok {
					t.Fatal("art canvas is not accessible")
				}
				for event := range 3 {
					if !sdl.TestPointerEvent(r.X+r.W/2, r.Y+r.H/2, event) {
						t.Fatal("SDL paint input failed")
					}
					step()
				}
				if !doc.Dirty() || model.History.Cursor() != 1 {
					t.Fatal("real canvas did not retain one dirty stroke")
				}
				pixels, _, err := model.Capture(frame, art.Albedo)
				if err != nil {
					t.Fatal(err)
				}
				panel, _ := ws.Panel(panelID)
				opened = append(opened, state{doc, model, panel, panelID, frame, bytes.Clone(pixels.Pix), model.Session.Document.Revision()})
				ws.ToggleMaximize(panelID)
			}
			assertDocuments := func() {
				t.Helper()
				if !slices.Equal(ed.Selected(), selected) {
					t.Fatal("workspace switch lost the scene selection")
				}
				for _, saved := range opened {
					panel, ok := ws.Panel(saved.panelID)
					if !ok || panel != saved.panel || ed.Docs.AssetDocument(saved.doc.Asset) != saved.doc || saved.doc.Binding.Model != saved.model || !saved.doc.Dirty() || saved.model.History.Cursor() != 1 || saved.model.Session.Frame != saved.frame || saved.model.Session.Document.Revision() != saved.revision {
						t.Fatal("workspace switch replaced a dirty document, view, frame or Undo history")
					}
					pixels, _, err := saved.model.Capture(saved.frame, art.Albedo)
					if err != nil || !bytes.Equal(pixels.Pix, saved.pixels) {
						t.Fatal("workspace switch changed unsaved pixels", err)
					}
				}
			}
			capture := func(name string) *image.RGBA {
				t.Helper()
				var img *image.RGBA
				var captureErr error
				s.Engine.CaptureFrame(func(pixels *image.RGBA, err error) { img, captureErr = pixels, err })
				step()
				if captureErr != nil || img == nil {
					t.Fatal("workspace screenshot failed", captureErr)
				}
				if root := os.Getenv("AEGO_ART_WORKSPACE_SHOTS"); root != "" {
					dir := filepath.Join(root, profile.name)
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					file, err := os.Create(filepath.Join(dir, name+".png"))
					if err != nil {
						t.Fatal(err)
					}
					if err := png.Encode(file, img); err != nil {
						file.Close()
						t.Fatal(err)
					}
					if err := file.Close(); err != nil {
						t.Fatal(err)
					}
				}
				return img
			}
			first := opened[0]
			ws.Focus(first.panelID)
			ws.ToggleMaximize(first.panelID)
			for range 3 {
				step()
			}
			r, _ := ws.Tree.PanelRect(first.panelID)
			x, y := r.X+r.W/2, r.Y+r.H/2
			if !sdl.TestWheelEvent(x, y, 1) {
				t.Fatal("SDL zoom input failed")
			}
			step()
			for _, event := range []struct {
				x, y float32
				kind int
			}{{x, y, 0}, {x, y, 1}, {x + 24, y + 16, 0}, {x + 24, y + 16, 2}} {
				if !sdl.TestRightPointerEvent(event.x, event.y, event.kind) {
					t.Fatal("SDL pan input failed")
				}
				step()
			}
			before := capture("canvas-zoom-pan-before")
			ws.ToggleMaximize(first.panelID)
			ws.Switch(workspace.Design)
			ws.Show(first.panelID)
			ws.Focus(first.panelID)
			if root, ok := ws.Tree.Node(ws.Tree.Root()); ok && root.Kind == dock.KindSplit {
				root.Ratio = .27
			}
			user, err := ws.SaveAs("My art workspace")
			if err != nil {
				t.Fatal(err)
			}
			userTree := ws.Tree.Save()
			if err := ws.SaveLayout(); err != nil {
				t.Fatal(err)
			}
			for i, name := range []string{workspace.Design, workspace.Art, workspace.Animation, workspace.VFX, workspace.Programming, workspace.Audio, workspace.Default, workspace.Design} {
				if !ws.Switch(name) {
					t.Fatal("factory workspace is unavailable", name)
				}
				if name == workspace.Animation {
					for range 2 {
						step()
					}
					capture("animation-authoring-before-play")
					if err := s.App.Play().Start(sceneDoc); err != nil {
						t.Fatal(err)
					}
					defer s.App.Play().Stop()
				} else if name == workspace.VFX {
					s.App.Play().Stop()
				}
				for range 3 {
					step()
				}
				assertDocuments()
				capture(fmt.Sprintf("%02d-%s", i, name))
			}
			if !ws.Switch(user) || !bytes.Equal(ws.Tree.Save(), userTree) {
				t.Fatal("factory workspace visits overwrote a named user layout")
			}
			ws.Switch(workspace.Art)
			ws.ResetLayout()
			assertDocuments()
			if !ws.Switch(user) || !bytes.Equal(ws.Tree.Save(), userTree) {
				t.Fatal("Reset Workspace overwrote a different user's layout")
			}
			// The same workspace must be used for the pixel comparison. A custom
			// layout intentionally uses inline properties/timeline when it has no
			// dedicated Art panels, so its canvas has a different viewport size.
			ws.Switch(workspace.Art)
			ws.Show(first.panelID)
			ws.Focus(first.panelID)
			if !ws.ToggleMaximize(first.panelID) {
				t.Fatal("cannot reopen the retained canvas")
			}
			for range 3 {
				step()
			}
			after := capture("canvas-zoom-pan-after")
			// Compare actual canvas pixels, excluding changing tab/hover status.
			crop := mathx.Rect{X: r.X + 8, Y: r.Y + 90, W: r.W - 16, H: r.H - 130}
			for y := int(crop.Y); y < int(crop.Y+crop.H); y++ {
				for x := int(crop.X); x < int(crop.X+crop.W); x++ {
					if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
						t.Fatalf("workspace switch reset zoom/pan or changed the retained canvas at %d,%d", x, y)
					}
				}
			}
			assertDocuments()
			t.Logf("%dx%d density %.1f: two dirty documents, eight frames, selection, Undo, user layout and pixel-identical zoom/pan survived all seven workspaces, Play/Stop and Reset Workspace", profile.w, profile.h, profile.density)
		})
	}
}
