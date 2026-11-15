//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	gameexport "aego/app/export"
	"aego/art"
	"aego/asset/importers/artsource"
	"aego/core/ids"
	"aego/editor/core/artedit"
	"aego/editor/core/assetops"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/engine/problems"
	"aego/modules/animation"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/platform/sdl"
	"aego/project"
	"aego/runtime/bundle"
	"aego/scene"
	"aego/schema"
)

// Restore the example's unchanged native module and go.mod only in its test
// copy. A temporary Go workspace points to this checkout; no project go.mod
// or Go toolchain is edited. The resulting export includes the real module.
func preparePlatformerNativeExportProject(t *testing.T, p *project.Project) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"go.mod", "native/module.go"} {
		data, err := os.ReadFile(filepath.Join(root, "examples", "platformer", rel))
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(p.Root, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	work := filepath.Join(t.TempDir(), "go.work")
	content := fmt.Sprintf("go 1.27\n\nuse (\n\t%q\n\t%q\n)\n", root, p.Root)
	if err := os.WriteFile(work, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOWORK", work)
	p.Native = true
}

func platformerSlicedPNGGameUndoExport(t *testing.T, p *project.Project, s *Session, sceneDoc *document.SceneDocument, artDoc *document.AssetDocument, model *artedit.Document, original, painted []byte, key func(input.Scancode, bool)) {
	t.Helper()
	ed, ws := s.App.Context(), s.App.Workspace()
	panelID := "render2d.art/" + artDoc.Asset.String()
	info := model.Session.Document.Info()
	firstFrame := artsource.StableID(artDoc.Asset, artsource.FrameName(info.Frames[0].ID, art.Albedo))
	secondFrame := artsource.StableID(artDoc.Asset, artsource.FrameName(info.Frames[1].ID, art.Albedo))
	clipID := artsource.StableID(artDoc.Asset, artsource.ClipName(""))

	waitPreview := func(previous uint64) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if artDoc.WorkingPreview.Error != "" {
				t.Fatalf("art preview failed: %+v", artDoc.WorkingPreview)
			}
			if artDoc.WorkingPreview.Applied != previous && !artDoc.WorkingPreview.Pending {
				return
			}
			if !s.Step() {
				t.Fatal("editor stopped preparing the Player preview")
			}
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("art preview did not finish: %+v", artDoc.WorkingPreview)
	}
	redo := func() {
		t.Helper()
		if !sdl.TestEditKey(input.KeyZ, true, true, true) || !s.Step() || !sdl.TestEditKey(input.KeyZ, false, false, false) || !s.Step() {
			t.Fatal("native Redo input failed")
		}
	}
	previous := artDoc.WorkingPreview.Applied
	redo()
	waitPreview(previous)

	// A copied controller preserves all original states but explicitly points
	// its Idle state to the new editable animation. The actual scene is changed
	// through its normal scene history; the original source files stay intact.
	controllerPath := filepath.Join(p.Root, "assets", "anim", "player.aectrl")
	controller, err := os.ReadFile(controllerPath)
	if err != nil {
		t.Fatal(err)
	}
	oldClip := `state "Idle" clip=asset("a0a10000000000000000000000000001")`
	newClip := `state "Idle" clip=asset("` + clipID.String() + `")`
	if bytes.Count(controller, []byte(oldClip)) != 1 {
		t.Fatal("the example's Idle controller is not the expected source")
	}
	copyController := bytes.Replace(controller, []byte(oldClip), []byte(newClip), 1)
	controllerID, err := assetops.CreateFile(ed, "assets/anim/player-art-copy.aectrl", copyController)
	if err != nil {
		t.Fatalf("create an independent Player controller: %v", err)
	}
	sc := sceneDoc.Scene()
	hero := sc.Find(sc.Root(), "Player")
	td, ok := sc.Registry().ByName("animation.Animator")
	if !ok {
		t.Fatal("Animator is not registered")
	}
	fd, ok := td.Field("controller")
	if !ok {
		t.Fatal("Animator controller field is not registered")
	}
	if err := sceneDoc.History.Exec(commands.SetField(sc, sc.ID(hero), td, fd, schema.Asset(controllerID), "Animator controller")); err != nil {
		t.Fatalf("explicitly assign the copied controller: %v", err)
	}
	if !sceneDoc.Dirty() {
		t.Fatal("explicit controller assignment did not enter scene history")
	}
	if err := ed.Docs.Save(sceneDoc.ID); err != nil {
		t.Fatalf("save the explicitly edited test scene: %v", err)
	}
	exporter := s.App.AgentExporter()
	if exporter == nil {
		t.Fatal("editor export service is unavailable")
	}
	dirtyBlocked := false
	for _, d := range exporter.Check(exporter.Prepare(), "release") {
		if d.Code == gameexport.CodeDirtySource && d.Severity == problems.Error && d.Subject.File == artDoc.Path {
			dirtyBlocked = true
		}
	}
	if !dirtyBlocked {
		t.Fatal("export did not reject the unsaved Player drawing")
	}
	if err := s.App.Play().Start(sceneDoc); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	seen := map[ids.AssetID]bool{}
	playingFirst := false
	for time.Now().Before(deadline) {
		if !s.Step() {
			t.Fatal("editor stopped during Player animation")
		}
		playScene := s.App.Play().Scene()
		player := playScene.Find(playScene.Root(), "Player")
		a, haveAnimator := scene.Get[animation.Animator](playScene, player)
		sp, haveSprite := scene.Get[render2d.SpriteRenderer](playScene, player)
		if haveAnimator && haveSprite && a.Controller.ID == controllerID && a.State == "Idle" {
			seen[sp.Sprite.ID] = true
			if seen[firstFrame] && seen[secondFrame] && sp.Sprite.ID == firstFrame {
				playingFirst = true
				break
			}
		}
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	if !playingFirst {
		t.Fatalf("the real Player Animator did not cycle through both new frames: %+v", seen)
	}
	s.App.Play().Pause()
	view := func(label string) ([]byte, uint64) {
		t.Helper()
		if current := ws.Tree.Maximized(); current != "" && current != workspace.Game {
			ws.ToggleMaximize(current)
		}
		ws.Show(workspace.Game)
		ws.Focus(workspace.Game)
		if ws.Tree.Maximized() != workspace.Game && !ws.ToggleMaximize(workspace.Game) {
			t.Fatal("could not maximize Game View")
		}
		for range 4 {
			if !s.Step() {
				t.Fatal("editor stopped before Game View capture")
			}
		}
		image, tick, path, err := s.App.CaptureAgentView("game")
		if err != nil || path != sceneDoc.Path {
			t.Fatalf("capture Player Game View: path=%s, err=%v", path, err)
		}
		if dir := os.Getenv("AEGO_PNG_FRAMES_SHOTS"); dir != "" {
			if err := os.WriteFile(filepath.Join(dir, "game-"+label+".png"), image, 0644); err != nil {
				t.Fatal(err)
			}
		}
		ws.ToggleMaximize(workspace.Game)
		ws.Focus(panelID)
		return image, tick
	}
	gamePainted, paintedTick := view("painted")
	previous = artDoc.WorkingPreview.Applied
	key(input.KeyZ, true)
	waitPreview(previous)
	pixels, _, err := model.Capture(info.Frames[0].ID, art.Albedo)
	if err != nil || !bytes.Equal(pixels.Pix, original) {
		t.Fatalf("Undo during Play did not restore the source pixels: %v", err)
	}
	gameUndone, undoneTick := view("undo")
	previous = artDoc.WorkingPreview.Applied
	redo()
	waitPreview(previous)
	pixels, _, err = model.Capture(info.Frames[0].ID, art.Albedo)
	if err != nil || !bytes.Equal(pixels.Pix, painted) {
		t.Fatalf("Redo during Play did not restore the source pixels: %v", err)
	}
	gameRedone, redoneTick := view("redo")
	if paintedTick != undoneTick || paintedTick != redoneTick || bytes.Equal(gamePainted, gameUndone) || !bytes.Equal(gamePainted, gameRedone) {
		t.Fatalf("Game View did not follow Player Undo/Redo at the same tick: %d, %d, %d", paintedTick, undoneTick, redoneTick)
	}
	s.App.Play().Stop()
	if err := ed.SaveAsset(artDoc); err != nil || artDoc.Dirty() {
		t.Fatalf("save the editable Player copy: dirty=%v, %v", artDoc.Dirty(), err)
	}
	if got, err := os.ReadFile(controllerPath); err != nil || !bytes.Equal(got, controller) {
		t.Fatalf("the original Player controller was changed: %v", err)
	}
	out := filepath.Join(t.TempDir(), "game-export")
	report, err := exporter.Export(exporter.Prepare(), services.ExportRequest{Target: "host", Profile: "release", Out: out}, nil)
	if err != nil || report.Blocked {
		t.Fatalf("actual native export failed: %+v, %v", report, err)
	}
	manifest, err := os.ReadFile(filepath.Join(out, bundle.DataDir, bundle.ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	data, err := bundle.DecodeData(manifest, bundle.ManifestFile)
	if err != nil {
		t.Fatal(err)
	}
	sceneID, ok := s.Pipeline.Lookup(sceneDoc.Path)
	if !ok {
		t.Fatal("the exported scene has no asset ID")
	}
	for _, id := range []ids.AssetID{artDoc.Asset, firstFrame, clipID, controllerID, sceneID} {
		live, ok := s.Pipeline.Entry(id)
		if !ok {
			t.Fatalf("editor pipeline lost %s", id)
		}
		exported, ok := data.Index.Lookup(id)
		if !ok || len(exported.Artifacts) == 0 {
			t.Fatalf("export did not include asset %s", id)
		}
		for _, artifact := range exported.Artifacts {
			local, ok := live.Artifact(artifact.Kind)
			if !ok || !strings.HasPrefix(artifact.Path, "data://") {
				t.Fatalf("exported artifact has no matching editor artifact: %+v", artifact)
			}
			want, err := s.Pipeline.Artifacts().ReadFile(local.Path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(out, bundle.DataDir, strings.TrimPrefix(artifact.Path, "data://")))
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("exported bytes differ for %s/%s: %v", id, artifact.Kind, err)
			}
		}
	}
	binary, err := gameexport.Launchable(p, "host", out)
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, binary, "--headless", "--frames", "120")
	cmd.Env = append(os.Environ(), "SDL_AUDIODRIVER=dummy", "SDL_AUDIO_DRIVER=dummy", "XDG_CONFIG_HOME="+t.TempDir(), "XDG_DATA_HOME="+t.TempDir())
	output, err := cmd.CombinedOutput()
	if err != nil || bytes.Contains(output, []byte("script stopped:")) || bytes.Contains(output, []byte(" ERROR ")) {
		t.Fatalf("exported native game did not run cleanly: %v\n%s", err, output)
	}
}
