//go:build linux

package edit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/importers/scenefile"
	"aego/core/ids"
	"aego/editor/core/artedit"
	"aego/editor/core/document"
	"aego/editor/core/selection"
	"aego/editor/core/state"
	"aego/editor/core/workspace"
	"aego/modules/animation"
	"aego/modules/render2d"
	"aego/project"
	"aego/scene"
	"aego/scene/sceneio"
)

const artCrashReady = "AEGO_ART_RECOVERY_WRITTEN"

// This uses a temporary project and a real headless editor subprocess. The
// parent kills it after a recovery snapshot has been written, then explicitly
// requests recovery; it does not simulate a power failure or multi-file save.
func TestArtRecoveryAfterForcedProcessTermination(t *testing.T) {
	if root := os.Getenv("AEGO_ART_CRASH_TEST_ROOT"); root != "" {
		artRecoveryCrashChild(t, root)
		return
	}
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	t.Setenv("SDL_AUDIO_DRIVER", "dummy")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, conflict := range []bool{false, true} {
		name := "recover"
		if conflict {
			name = "external source conflict"
		}
		t.Run(name, func(t *testing.T) {
			p := newProject(t)
			id := ids.NewAssetID()
			drawing, err := art.New(16, 16, art.RGBA, art.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			original, err := drawing.Encode()
			drawing.Close()
			if err != nil {
				t.Fatal(err)
			}
			const rel = "assets/recovery.aeart"
			file := filepath.Join(p.Root, filepath.FromSlash(rel))
			if err := os.WriteFile(file, original, 0644); err != nil {
				t.Fatal(err)
			}
			imp := artsource.New()
			meta := asset.NewSourceMeta(imp.ID(), imp.Version(), imp.Defaults())
			meta.ID = id
			if err := os.WriteFile(file+".meta", asset.MarshalSourceMeta(meta), 0644); err != nil {
				t.Fatal(err)
			}
			const scenePath = "scenes/recovery.aescene"
			sceneID := ids.NewAssetID()
			var selectedNode ids.NodeID
			writeScene(t, p, scenePath, func(sc *scene.Scene) {
				node := sc.Create(sc.Root(), "Selected hero")
				selectedNode = sc.ID(node)
				*scene.Add[render2d.SpriteRenderer](sc, node) = render2d.DefaultSpriteRenderer()
				sprite, _ := scene.Get[render2d.SpriteRenderer](sc, node)
				sprite.Sprite.ID = id
				player := animation.DefaultPlayer()
				player.Clip.ID = artsource.StableID(id, artsource.ClipName(""))
				*scene.Add[animation.AnimationPlayer](sc, node) = player
			})
			sceneFile := filepath.Join(p.Root, filepath.FromSlash(scenePath))
			sceneImporter := scenefile.NewScene()
			sceneMeta := asset.NewSourceMeta(sceneImporter.ID(), sceneImporter.Version(), sceneImporter.Defaults())
			sceneMeta.ID = sceneID
			if err := os.WriteFile(sceneFile+".meta", asset.MarshalSourceMeta(sceneMeta), 0644); err != nil {
				t.Fatal(err)
			}
			p.MainScene = sceneID
			if err := p.Save(); err != nil {
				t.Fatal(err)
			}
			// Represent the last saved editor session, as on reopening a project
			// after a crash. Recovery must not replace its scene selection.
			editorState := state.Default()
			editorState.Workspace, editorState.Active = workspace.Art, scenePath
			editorState.Docs = []state.Doc{{Path: scenePath, Selection: []ids.NodeID{selectedNode}}}
			if err := state.Save(p.Root, editorState); err != nil {
				t.Fatal(err)
			}
			sceneDisk, err := os.ReadFile(sceneFile)
			if err != nil {
				t.Fatal(err)
			}
			projectDisk, err := os.ReadFile(filepath.Join(p.Root, project.FileName))
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestArtRecoveryAfterForcedProcessTermination$")
			cmd.Env = append(os.Environ(), "AEGO_ART_CRASH_TEST_ROOT="+p.Root, "AEGO_ART_CRASH_TEST_ID="+id.String())
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			ready := make(chan error, 1)
			go func() {
				scan := bufio.NewScanner(stdout)
				for scan.Scan() {
					if scan.Text() == artCrashReady {
						ready <- nil
						return
					}
				}
				ready <- fmt.Errorf("child exited before writing recovery: %v", scan.Err())
			}()
			select {
			case err := <-ready:
				if err != nil {
					_ = cmd.Wait()
					t.Fatalf("%v: %s", err, stderr.String())
				}
			case <-ctx.Done():
				_ = cmd.Wait()
				t.Fatalf("child timed out before recovery: %v; %s", ctx.Err(), stderr.String())
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = stdin.Close()
			if err := cmd.Wait(); err == nil || cmd.ProcessState.Success() {
				t.Fatal("editor subprocess was not forcibly terminated")
			}
			if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, original) {
				t.Fatal("the unsaved drawing changed the source before recovery")
			}
			// An older, unrelated scene recovery may be offered alongside the
			// Art recovery. It must not become the active scene on restore.
			const otherScenePath = "scenes/unknown.aescene"
			otherRecoveryFile := filepath.Join(p.Root, document.RecoveryDir, hex.EncodeToString([]byte(otherScenePath))+document.SceneExt)
			if err := os.WriteFile(otherRecoveryFile, sceneDisk, 0600); err != nil {
				t.Fatal(err)
			}
			if conflict {
				other, err := art.New(8, 8, art.RGBA, art.DefaultLimits())
				if err != nil {
					t.Fatal(err)
				}
				original, err = other.Encode()
				other.Close()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, original, 0644); err != nil {
					t.Fatal(err)
				}
			}
			s, err := Open(p, headless())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if !s.Step() {
				t.Fatal("editor stopped on reopen")
			}
			ed := s.App.Context()
			sceneDoc := ed.Docs.Edit()
			if sceneDoc == nil || sceneDoc.Path != scenePath {
				t.Fatalf("startup scene was not opened: %+v", sceneDoc)
			}
			sc := sceneDoc.Scene()
			hero := sc.Find(sc.Root(), "Selected hero")
			item := selection.Item{Doc: sceneDoc.ID, Node: sc.ID(hero)}
			if item.Node != selectedNode {
				t.Fatal("the selected scene node did not keep its identity across restart")
			}
			sceneBefore := sceneio.Save(sc)
			historyBefore := sceneDoc.History.Cursor()
			assertScenePreserved := func(stage string) {
				t.Helper()
				selected, ok := ed.Selection.Active()
				if !ok || selected != item || ed.Docs.Edit() != sceneDoc || sceneDoc.Scene() != sc ||
					sceneDoc.Path != scenePath || sceneDoc.Dirty() || sceneDoc.History.Cursor() != historyBefore ||
					!bytes.Equal(sceneBefore, sceneio.Save(sc)) {
					t.Fatalf("%s changed the active scene, selection, history or scene contents", stage)
				}
				sprite, spriteOK := scene.Get[render2d.SpriteRenderer](sc, hero)
				player, playerOK := scene.Get[animation.AnimationPlayer](sc, hero)
				if !spriteOK || sprite.Sprite.ID != id || !playerOK || player.Clip.ID != artsource.StableID(id, artsource.ClipName("")) {
					t.Fatalf("%s changed the selected hero's art references", stage)
				}
				if unknown, ok := scene.Get[sceneio.Unknown](sc, hero); ok && len(unknown.Components) != 0 {
					t.Fatalf("%s turned a known component into Unknown: %+v", stage, unknown.Components)
				}
				entry, ok := ed.Pipeline.Entry(sceneID)
				loaderEntry, loaded := ed.Loader.Index().Lookup(sceneID)
				if p.MainScene != sceneID || !ok || entry.Kind != asset.ArtScene || !loaded || loaderEntry.Kind != asset.ArtScene {
					t.Fatalf("%s lost the startup scene or its imported type", stage)
				}
				if disk, err := os.ReadFile(sceneFile); err != nil || !bytes.Equal(disk, sceneDisk) {
					t.Fatalf("%s changed the scene on disk: %v", stage, err)
				}
				if disk, err := os.ReadFile(filepath.Join(p.Root, project.FileName)); err != nil || !bytes.Equal(disk, projectDisk) {
					t.Fatalf("%s changed the project on disk: %v", stage, err)
				}
			}
			assertScenePreserved("startup")
			doc, err := ed.OpenAsset(id)
			if err != nil {
				t.Fatal(err)
			}
			model, ok := doc.Binding.Model.(*artedit.Document)
			if !ok || !model.PendingRecovery || doc.Dirty() {
				t.Fatal("recovery was not offered explicitly or was applied automatically")
			}
			recoveryFile := filepath.Join(p.Root, document.RecoveryDir, "asset-"+id.String()+".recovery")
			recoveryBefore, err := os.ReadFile(recoveryFile)
			if err != nil {
				t.Fatal(err)
			}
			assertScenePreserved("opening the pending drawing")
			sceneRecoveries := ed.Docs.FindRecovery()
			if len(sceneRecoveries) != 1 || sceneRecoveries[0].Path != otherScenePath {
				t.Fatalf("unrelated scene recovery was not offered: %+v", sceneRecoveries)
			}
			restoredScene, err := ed.Docs.RestoreRecovery(sceneRecoveries[0])
			if err != nil || restoredScene == nil || !restoredScene.Dirty() {
				t.Fatalf("explicit unrelated scene recovery failed: %v", err)
			}
			assertScenePreserved("recovering the unrelated scene")
			err = model.Recover() // This explicit call represents consent in the test.
			if conflict {
				if err == nil || !strings.Contains(err.Error(), "conflict") || !model.PendingRecovery {
					t.Fatalf("changed source did not block recovery: %v", err)
				}
			} else {
				if err != nil || !doc.Dirty() || model.PendingRecovery {
					t.Fatalf("explicit recovery did not restore a dirty document: %v", err)
				}
				img, _, err := model.Capture(model.Session.Frame, art.Albedo)
				if err != nil || img.NRGBAAt(4, 4).R != 255 || img.NRGBAAt(4, 4).A != 255 {
					t.Fatalf("recovered pixels differ: %v", err)
				}
				// Let the real editor's live-source coordinator publish the dirty
				// generation, not just the in-memory recovery call.
				deadline := time.Now().Add(8 * time.Second)
				for doc.WorkingPreview.Applied == 0 && doc.WorkingPreview.Error == "" && time.Now().Before(deadline) {
					if !s.Step() {
						t.Fatal("editor stopped while publishing recovered art")
					}
					runtime.Gosched()
					time.Sleep(time.Millisecond)
				}
				if doc.WorkingPreview.Applied == 0 || doc.WorkingPreview.Pending || doc.WorkingPreview.Error != "" {
					t.Fatalf("recovered art preview was not published: %+v", doc.WorkingPreview)
				}
			}
			assertScenePreserved("recovering the drawing and publishing its preview")
			if got, err := os.ReadFile(recoveryFile); err != nil || !bytes.Equal(got, recoveryBefore) {
				t.Fatalf("recovery changed its recovery copy without a save: %v", err)
			}
			if got, err := os.ReadFile(otherRecoveryFile); err != nil || !bytes.Equal(got, sceneDisk) {
				t.Fatalf("scene recovery changed its recovery copy without a save: %v", err)
			}
			if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, original) {
				t.Fatal("recovery or conflict handling overwrote the source")
			}
		})
	}
}

func artRecoveryCrashChild(t *testing.T, root string) {
	t.Helper()
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	id, err := ids.ParseAssetID(os.Getenv("AEGO_ART_CRASH_TEST_ID"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, headless())
	if err != nil {
		t.Fatal(err)
	}
	if !s.Step() {
		t.Fatal("editor stopped before editing")
	}
	doc, err := s.App.Context().OpenAsset(id)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok {
		t.Fatal("Art Studio did not open the source")
	}
	_, err = model.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 4, Y: 4}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}})
	if err != nil || !doc.Dirty() {
		t.Fatalf("could not create an unsaved stroke: %v", err)
	}
	if n, err := s.App.Context().Docs.WriteRecovery(); err != nil || n < 1 {
		t.Fatalf("recovery snapshot was not written: %d, %v", n, err)
	}
	fmt.Println(artCrashReady)
	// Keep the process alive for the parent to kill; no clean editor shutdown.
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}
