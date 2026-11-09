//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"aego/anim"
	"aego/art"
	"aego/asset"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/editor/core/workspace"
	"aego/modules/animation"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/platform/sdl"
	"aego/project"
	"aego/scene"
)

// These are evidence records, not another document format. The transferred
// source is the exact .aeart written by the normal editor Save command.
type artInteropState struct {
	Document     art.Info                          `json:"document"`
	FrameHashes  map[art.ID]map[art.Channel]string `json:"frameHashes"`
	SourceSHA256 string                            `json:"sourceSHA256"`
}

type artInteropWitness struct {
	Schema       int               `json:"schema"`
	State        artInteropState   `json:"state"`
	SourcePath   string            `json:"sourcePath"`
	ScenePath    string            `json:"scenePath"`
	ProjectFiles map[string]string `json:"projectFiles"`
	SourceAsset  string            `json:"sourceAsset"`
	SceneRefs    map[string]string `json:"sceneRefs"`
	SubAssets    map[string]string `json:"subAssets"`
	ClipSHA256   string            `json:"clipSHA256"`
	ClipFrames   []string          `json:"clipFrames"`
}

const interopArtPath = "assets/hero.aeart"
const interopScenePath = "scenes/art-workflow.aescene"

func interopSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func interopRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func interopWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
}

func interopJSON(t *testing.T, path string, value any) {
	t.Helper()
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	interopWrite(t, path, append(b, '\n'))
}

func interopState(t *testing.T, model *artedit.Document, saved []byte) artInteropState {
	t.Helper()
	state := artInteropState{Document: model.Session.Document.Info(), SourceSHA256: interopSHA(saved), FrameHashes: map[art.ID]map[art.Channel]string{}}
	for _, frame := range state.Document.Frames {
		state.FrameHashes[frame.ID] = map[art.Channel]string{}
		for _, channel := range []art.Channel{art.Albedo, art.Normal, art.Height, art.Emission} {
			// This is exactly the render/hash contract of aegoArt.hash(frame, channel).
			pixels, err := model.Session.Document.Render(frame.ID, art.RenderOptions{Channel: channel})
			if err != nil {
				t.Fatal(err)
			}
			state.FrameHashes[frame.ID][channel] = interopSHA(pixels.Pix)
		}
	}
	return state
}

func interopStep(t *testing.T, s *Session) {
	t.Helper()
	if !s.Step() {
		t.Fatal("native editor stopped during file interoperability")
	}
}

func interopClick(t *testing.T, s *Session, x, y float32) {
	t.Helper()
	for kind := range 3 {
		if !sdl.TestPointerEvent(x, y, kind) {
			t.Fatal("SDL file interoperability pointer failed")
		}
		interopStep(t, s)
	}
}

func interopKey(t *testing.T, s *Session, key input.Scancode, ctrl bool) {
	t.Helper()
	for _, down := range []bool{true, false} {
		if !sdl.TestEditKey(key, down, down && ctrl, false) {
			t.Fatal("SDL file interoperability key failed")
		}
		interopStep(t, s)
	}
}

func interopMaximize(t *testing.T, s *Session, panel string) mathx.Rect {
	t.Helper()
	ws := s.App.Workspace()
	ws.Tree.Restore()
	ws.Show(panel)
	ws.Focus(panel)
	if !ws.ToggleMaximize(panel) {
		t.Fatal("cannot show interoperability panel", panel)
	}
	for range 3 {
		interopStep(t, s)
	}
	r, ok := ws.Tree.PanelRect(panel)
	if !ok {
		t.Fatal("interoperability panel has no visible rectangle", panel)
	}
	return r
}

func interopShot(t *testing.T, s *Session, path string) {
	t.Helper()
	var pixels *image.RGBA
	var captureErr error
	s.Engine.CaptureFrame(func(img *image.RGBA, err error) { pixels, captureErr = img, err })
	interopStep(t, s)
	if pixels == nil || captureErr != nil {
		t.Fatal("native interoperability screenshot failed", captureErr)
	}
	var data bytes.Buffer
	if err := png.Encode(&data, pixels); err != nil {
		t.Fatal(err)
	}
	interopWrite(t, path, data.Bytes())
}

func interopWitness(t *testing.T, s *Session, model *artedit.Document) artInteropWitness {
	t.Helper()
	ed := s.App.Context()
	source := interopRead(t, filepath.Join(s.Project.Root, interopArtPath))
	meta, err := asset.ParseSourceMeta(interopRead(t, filepath.Join(s.Project.Root, interopArtPath+".meta")), interopArtPath+".meta")
	if err != nil {
		t.Fatal(err)
	}
	w := artInteropWitness{Schema: 1, State: interopState(t, model, source), SourcePath: interopArtPath, ScenePath: interopScenePath,
		SourceAsset: meta.ID.String(), SceneRefs: map[string]string{}, SubAssets: map[string]string{}}
	if len(w.State.Document.Frames) != 8 {
		t.Fatal("file interoperability requires the eight authored frames")
	}
	sceneDoc := ed.Docs.Find(interopScenePath)
	if sceneDoc == nil {
		t.Fatal("the actual assigned source scene is not open")
	}
	sc := sceneDoc.Scene()
	hero := sc.Find(sc.Root(), "Hero")
	sprite, hasSprite := scene.Get[render2d.SpriteRenderer](sc, hero)
	player, hasPlayer := scene.Get[animation.AnimationPlayer](sc, hero)
	if !hasSprite || !hasPlayer || sprite.Sprite.ID.IsZero() || player.Clip.ID.IsZero() || sprite.Sprite.ID != meta.ID {
		t.Fatal("file interoperability lost the real SpriteRenderer/AnimationPlayer assignment")
	}
	w.SceneRefs["SpriteRenderer.sprite"], w.SceneRefs["AnimationPlayer.clip"] = sprite.Sprite.ID.String(), player.Clip.ID.String()
	for _, sub := range meta.Subs {
		if err := ed.Pipeline.VerifyCurrent(sub.ID); err != nil {
			t.Fatal("saved source subasset is unavailable", sub.Name, err)
		}
		w.SubAssets[sub.Name] = sub.ID.String()
	}
	var clip *anim.Clip
	var clipBytes []byte
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if data, ok := ed.Loader.Anim(player.Clip.ID); ok {
			if value, err := anim.Unmarshal(data); err == nil && len(value.Tracks) == 1 && len(value.Tracks[0].Keys) == 8 {
				clip, clipBytes = value, data
				break
			}
		}
		interopStep(t, s)
		runtime.Gosched()
	}
	if clip == nil {
		t.Fatal("assigned runtime clip did not publish eight prepared frames after Save")
	}
	clipEntry, ok := ed.Pipeline.Entry(player.Clip.ID)
	if !ok {
		t.Fatal("assigned runtime clip is absent from the persistent index")
	}
	w.SubAssets[clipEntry.Name] = player.Clip.ID.String()
	w.ClipSHA256 = interopSHA(clipBytes)
	for _, key := range clip.Tracks[0].Keys {
		if key.V.A.IsZero() {
			t.Fatal("runtime clip has an empty frame AssetRef")
		}
		if err := ed.Pipeline.VerifyCurrent(key.V.A); err != nil {
			t.Fatal("runtime frame AssetRef does not resolve", err)
		}
		entry, ok := ed.Pipeline.Entry(key.V.A)
		if !ok {
			t.Fatal("runtime frame is absent from the persistent index")
		}
		w.SubAssets[entry.Name] = key.V.A.String()
		w.ClipFrames = append(w.ClipFrames, key.V.A.String())
	}
	if len(w.SubAssets) < 9 {
		t.Fatal("the actual persistent clip/frame identities were not recorded")
	}
	return w
}

func artFileInteropSourceNative(t *testing.T, s *Session, panelID string) {
	t.Helper()
	id, ok := s.Pipeline.Lookup(interopArtPath)
	if !ok {
		t.Fatal("authoring workflow lost its source")
	}
	doc := s.App.Context().Docs.AssetDocument(id)
	model := doc.Binding.Model.(*artedit.Document)
	interopMaximize(t, s, panelID)
	// The existing workflow just proved a two-frame dirty preview. Return to
	// its saved, SDL-painted first frame through common Undo, then author all
	// seven new frames with real Timeline controls, including a linked pair.
	for n := 0; len(model.Session.Document.Info().Frames) > 1 && n < 8; n++ {
		interopKey(t, s, input.KeyZ, true)
	}
	if len(model.Session.Document.Info().Frames) != 1 {
		t.Fatal("common Undo did not return to the SDL-painted source frame")
	}
	r := interopMaximize(t, s, "art.timeline")
	ui := s.App.Context().UI
	pad, row, gap := ui.Theme.Padding, ui.Theme.RowHeight, ui.Theme.Spacing
	width, y := r.W-2*pad, r.Y+pad+(row+gap)+row/2
	for range 6 {
		interopClick(t, s, r.X+pad+width*3/8, y) // Duplicate: independent image.
	}
	interopClick(t, s, r.X+pad+width*5/8, y) // Link: shared image.
	info := model.Session.Document.Info()
	if len(info.Frames) != 8 {
		t.Fatal("native Timeline did not create eight frames")
	}
	linked := map[art.ID]int{}
	for _, cel := range info.Cels {
		linked[cel.Image]++
	}
	hasLinked := false
	for _, count := range linked {
		hasLinked = hasLinked || count > 1
	}
	if !hasLinked {
		t.Fatal("native Link control did not preserve shared cel identity")
	}
	interopMaximize(t, s, panelID)
	interopKey(t, s, input.KeyS, true)
	if doc.Dirty() {
		t.Fatal("SDL Ctrl+S did not save the native eight-frame document")
	}
	saved := interopRead(t, filepath.Join(s.Project.Root, interopArtPath))
	encoded, err := model.Session.Document.Encode()
	if err != nil || !bytes.Equal(saved, encoded) {
		t.Fatal("normal Save did not persist the exact current Art bytes", err)
	}
	w := interopWitness(t, s, model)
	if out := os.Getenv("AEGO_ART_INTEROP_OUT"); out != "" {
		if err := os.Mkdir(out, 0755); err != nil {
			t.Fatal("use a new interoperability evidence directory", err)
		}
		w.ProjectFiles = map[string]string{}
		err := filepath.WalkDir(s.Project.Root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(s.Project.Root, path)
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if rel == ".aego" || rel == "exports" {
					return filepath.SkipDir
				}
				return os.MkdirAll(filepath.Join(out, "project", rel), 0755)
			}
			if !entry.Type().IsRegular() {
				t.Fatal("unexpected non-regular file in the native fixture", rel)
			}
			data := interopRead(t, path)
			rel = filepath.ToSlash(rel)
			w.ProjectFiles[rel] = interopSHA(data)
			interopWrite(t, filepath.Join(out, "project", filepath.FromSlash(rel)), data)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		interopJSON(t, filepath.Join(out, "native-source.json"), w)
		interopShot(t, s, filepath.Join(out, "native-source.png"))
	}
	t.Log("file interoperability producer: actual SDL-painted source, six Timeline Duplicates, one Link, Ctrl+S; eight frames, live SpriteRenderer/AnimationPlayer and current prepared AssetRefs")
}

func artFileInteropReturnNative(t *testing.T, root string) {
	t.Helper()
	t.Setenv("AEGO_LANG", "en")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var before artInteropWitness
	if err := json.Unmarshal(interopRead(t, filepath.Join(root, "native-source.json")), &before); err != nil {
		t.Fatal(err)
	}
	if before.Schema != 1 || before.SourcePath != interopArtPath || before.ScenePath != interopScenePath || len(before.ProjectFiles) < 4 {
		t.Fatal("invalid native interoperability witness")
	}
	var web struct {
		Status         string          `json:"status"`
		State          artInteropState `json:"state"`
		InputSHA256    string          `json:"inputSHA256"`
		DownloadSHA256 string          `json:"downloadSHA256"`
	}
	if err := json.Unmarshal(interopRead(t, filepath.Join(root, "browser.json")), &web); err != nil || web.Status != "PASS" || web.InputSHA256 != before.State.SourceSHA256 || web.DownloadSHA256 != web.State.SourceSHA256 {
		t.Fatal("native return requires a successful exact-file browser witness", err)
	}
	projectRoot := filepath.Join(root, "project")
	for path, expected := range before.ProjectFiles {
		if path == before.SourcePath {
			expected = web.DownloadSHA256
		}
		if interopSHA(interopRead(t, filepath.Join(projectRoot, filepath.FromSlash(path)))) != expected {
			t.Fatal("returned project changed a file other than the downloaded Art source", path)
		}
	}
	p, err := project.Load(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	interopStep(t, s)
	ed := s.App.Context()
	if _, err := ed.Docs.Open(before.ScenePath); err != nil {
		t.Fatal(err)
	}
	id, ok := s.Pipeline.Lookup(before.SourcePath)
	if !ok || id.String() != before.SourceAsset {
		t.Fatal("returned source lost its original AssetID")
	}
	interopMaximize(t, s, workspace.Assets)
	s.App.AssetsPanel().SetFolder("assets")
	for range 3 {
		interopStep(t, s)
	}
	tile, _, visible := s.App.AssetsPanel().TileRect(id)
	if !visible || ed.Docs.AssetDocument(id) != nil {
		t.Fatal("returned source was not a closed, visible Assets tile")
	}
	failOpen := func(stage string) {
		t.Helper()
		current, _, tileVisible := s.App.AssetsPanel().TileRect(id)
		d := ed.Docs.AssetDocument(id)
		dirty, handler := false, ""
		var model any
		if d != nil {
			dirty, handler, model = d.Dirty(), d.Handler, d.Binding.Model
		}
		notice, _ := ed.LastNotice()
		ws := s.App.Workspace()
		interopShot(t, s, filepath.Join(root, "native-return-failure.png"))
		t.Fatalf("%s: docNil=%t dirty=%t handler=%q model=%T artVisible=%t workspace=%q maximized=%q focused=%q originalTile=%+v currentTile=%+v tileVisible=%t notice=%+v",
			stage, d == nil, dirty, handler, model, ws.Visible("render2d.art/"+id.String()), ws.Current().ID, ws.Tree.Maximized(), ws.Focused(), tile, current, tileVisible, notice)
	}
	interopClick(t, s, tile.X+tile.W/2, tile.Y+tile.H/2)
	currentTile, _, tileVisible := s.App.AssetsPanel().TileRect(id)
	if s.App.Workspace().Tree.Maximized() != workspace.Assets || !tileVisible || currentTile != tile {
		failOpen("single Assets click changed the explicitly maximized layout or tile")
	}
	// A real user double-clicks at the same position. Do not reacquire or
	// compensate for a moved tile: automatic inspection must not move it.
	interopClick(t, s, tile.X+tile.W/2, tile.Y+tile.H/2)
	doc := ed.Docs.AssetDocument(id)
	if doc == nil || doc.Dirty() || !s.App.Workspace().Visible("render2d.art/"+id.String()) {
		failOpen("actual Assets double-click did not open the downloaded Art document")
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok {
		t.Fatal("returned source opened a viewer instead of the Art Canvas")
	}
	after := interopWitness(t, s, model)
	if !reflect.DeepEqual(after.State, web.State) || after.SourceAsset != before.SourceAsset || !reflect.DeepEqual(after.SceneRefs, before.SceneRefs) || !reflect.DeepEqual(after.SubAssets, before.SubAssets) || !reflect.DeepEqual(after.ClipFrames, before.ClipFrames) || after.ClipSHA256 != before.ClipSHA256 {
		t.Fatal("native reopen changed browser pixels, stable document topology, durations, palette, pivot or runtime AssetRefs")
	}
	if reflect.DeepEqual(after.State.FrameHashes, before.State.FrameHashes) {
		t.Fatal("returned document has no actual browser pixel edit")
	}
	canvasPanel := interopMaximize(t, s, "render2d.art/"+id.String())
	interopShot(t, s, filepath.Join(root, "native-reopened.png"))
	// The independently reopened document has its own zoom/pan. Locate the
	// actual rendered Canvas texture, then a visible opaque pixel of the
	// selected cel through read-only sampling; never reuse producer coordinates
	// or mutate the document to make an input target convenient.
	initial := after.State.FrameHashes
	interopKey(t, s, input.KeyE, false)
	ui := ed.UI
	canvas := model.Session.Document.Info().Canvas
	var displayed, clip mathx.Rect
	textures := 0
	for _, item := range s.App.UIList().TestDrawnItems(mathx.Rect{W: ui.Size().X, H: ui.Size().Y}, ui.Font(), "", canvas.Width, canvas.Height) {
		if item.Bounds.W <= float32(canvas.Width) || item.Bounds.H <= float32(canvas.Height) ||
			item.Bounds.X < canvasPanel.X || item.Bounds.Y < canvasPanel.Y ||
			item.Bounds.X+item.Bounds.W > canvasPanel.X+canvasPanel.W || item.Bounds.Y+item.Bounds.H > canvasPanel.Y+canvasPanel.H {
			continue
		}
		displayed, clip = item.Bounds, item.Clip
		textures++
	}
	if textures != 1 {
		t.Fatalf("expected one visible source-sized texture inside the actual Canvas, got %d", textures)
	}
	pixel, screenPoint := image.Pt(-1, -1), mathx.Vec2{}
	var pixelRect mathx.Rect
	var originalPaint art.Paint
	for y := 0; y < canvas.Height && pixel.X < 0; y++ {
		for x := 0; x < canvas.Width; x++ {
			sample, err := model.Session.SamplePaint(image.Pt(x, y))
			if err != nil {
				t.Fatal(err)
			}
			if sample.Color.A == 0 {
				continue
			}
			cell := mathx.Rect{X: displayed.X + float32(x)*displayed.W/float32(canvas.Width), Y: displayed.Y + float32(y)*displayed.H/float32(canvas.Height), W: displayed.W / float32(canvas.Width), H: displayed.H / float32(canvas.Height)}
			center := mathx.V2(cell.X+cell.W/2, cell.Y+cell.H/2)
			if !clip.Contains(center) || !canvasPanel.Contains(center) {
				continue
			}
			pixel, pixelRect, screenPoint, originalPaint = image.Pt(x, y), cell, center, sample
			break
		}
	}
	if pixel.X < 0 {
		t.Fatal("no visible opaque pixel of the actual selected cel is available to erase")
	}
	interopJSON(t, filepath.Join(root, "native-erase-target.json"), struct {
		SourcePixel                 image.Point `json:"sourcePixel"`
		ScreenPoint                 mathx.Vec2  `json:"screenPoint"`
		PixelRect, CanvasRect, Clip mathx.Rect
		Frame, Layer                art.ID
		Channel                     art.Channel
	}{pixel, screenPoint, pixelRect, displayed, clip, model.Session.Frame, model.Session.Layer, model.Session.Channel})
	interopClick(t, s, screenPoint.X, screenPoint.Y)
	erased, sampleErr := model.Session.SamplePaint(pixel)
	if sampleErr != nil || erased.Color.A != 0 || model.History.Cursor() != 1 || !doc.Dirty() || reflect.DeepEqual(interopState(t, model, nil).FrameHashes, initial) {
		interopShot(t, s, filepath.Join(root, "native-return-edit-failure.png"))
		t.Fatalf("returned source was not editable through its real native Canvas: pixel=%v screen=%v history=%d dirty=%t erased=%+v error=%v", pixel, screenPoint, model.History.Cursor(), doc.Dirty(), erased, sampleErr)
	}
	interopKey(t, s, input.KeyZ, true)
	restored, sampleErr := model.Session.SamplePaint(pixel)
	if sampleErr != nil || restored != originalPaint || model.History.Cursor() != 0 || doc.Dirty() || !reflect.DeepEqual(interopState(t, model, nil).FrameHashes, initial) {
		t.Fatal("common native Undo did not restore the exact browser-returned pixels")
	}
	// Only automatic selection preserves maximize. Explicit asset inspection
	// (the same callback as readonly document fallback) must still reveal the
	// Inspector, and explicit Inspector focus must retain its normal behavior.
	interopMaximize(t, s, workspace.Assets)
	ed.InspectAsset(id)
	interopStep(t, s)
	if s.App.Workspace().Tree.Maximized() != "" || !s.App.Workspace().Visible(workspace.Inspector) {
		t.Fatal("explicit asset inspection no longer reveals the Inspector")
	}
	interopMaximize(t, s, workspace.Assets)
	s.App.Workspace().Focus(workspace.Inspector)
	interopStep(t, s)
	if s.App.Workspace().Tree.Maximized() != "" || s.App.Workspace().Focused() != workspace.Inspector {
		t.Fatal("explicit Inspector focus no longer restores its visible layout")
	}
	for path, expected := range before.ProjectFiles {
		if path == before.SourcePath {
			expected = web.DownloadSHA256
		}
		if interopSHA(interopRead(t, filepath.Join(projectRoot, filepath.FromSlash(path)))) != expected {
			t.Fatal("reopening/Undo rewrote source, scene or metadata", path)
		}
	}
	interopJSON(t, filepath.Join(root, "native-return.json"), struct {
		Status     string            `json:"status"`
		Witness    artInteropWitness `json:"witness"`
		CommonUndo bool              `json:"commonUndo"`
	}{"PASS", after, true})
	t.Log("actual browser download -> same project path and .meta -> SDL Assets double-click -> native Canvas: complete document, all four channel pixels, eight frames, cel links and assigned runtime AssetRefs unchanged; real paint/Undo PASS")
}
