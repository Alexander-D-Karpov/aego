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
	"strings"
	"testing"
	"time"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/core/curve"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/editor/core/workspace"
	"aego/engine/i18n"
	"aego/modules/particles"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/platform/sdl"
	"aego/scene"
	"aego/ui/widgets"
)

// The drawing and preset controls use synthetic SDL input, and the displayed
// preset uses the real CPU particle preview. Opening documents uses Go APIs.
func TestVFXCanvasAndParticlePresetNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	p := newProject(t)
	drawing, err := art.New(16, 16, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	info := drawing.Info()
	edit, err := drawing.Begin(drawing.Revision(), "Prepare two particle frames")
	if err != nil {
		t.Fatal(err)
	}
	second, err := edit.AddFrame(info.Frames[0].ID, 180)
	if err != nil {
		t.Fatal(err)
	}
	for i, frame := range []art.ID{info.Frames[0].ID, second} {
		cel, err := edit.EnsureCel(info.Layers[0].ID, frame, art.Albedo)
		if err != nil {
			t.Fatal(err)
		}
		color := art.Color{R: 255, A: 255}
		if i == 1 {
			color = art.Color{G: 255, A: 255}
		}
		for y := range 16 {
			for x := range 16 {
				if err := edit.SetPixel(cel, x, y, art.Paint{Color: color}, nil, false); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	change, err := edit.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if err := change.Apply(); err != nil {
		t.Fatal(err)
	}
	source, err := drawing.Encode()
	drawing.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, "assets", "workspace.aeart"), source, 0644); err != nil {
		t.Fatal(err)
	}
	preset, err := particles.PresetSource("Dust")
	if err != nil {
		t.Fatal(err)
	}
	preset = bytes.Replace(preset, []byte("playOnStart=false"), []byte("playOnStart=true"), 1)
	preset = bytes.Replace(preset, []byte("loop=false"), []byte("loop=true"), 1)
	preset = bytes.Replace(preset, []byte("rate=0"), []byte("rate=40"), 1)
	if err := os.WriteFile(filepath.Join(p.Root, "assets", "workspace.aepreset"), preset, 0644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1600, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			s.Close()
		}
	}()
	if !s.Step() {
		t.Fatal("editor stopped during startup")
	}
	ed, ws := s.App.Context(), s.App.Workspace()
	if !ws.Switch(workspace.VFX) {
		t.Fatal("VFX workspace is unavailable")
	}
	artID, ok := ed.Pipeline.Lookup("assets/workspace.aeart")
	if !ok {
		t.Fatal("art source was not imported")
	}
	artDoc, err := ed.OpenAsset(artID)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := artDoc.Binding.Model.(*artedit.Document)
	if !ok {
		t.Fatal("art document was not bound")
	}
	presetID, ok := ed.Pipeline.Lookup("assets/workspace.aepreset")
	if !ok {
		t.Fatal("particle preset was not imported")
	}
	presetDoc, err := ed.OpenAsset(presetID)
	if err != nil || presetDoc.Handler != "particles.preset" {
		t.Fatalf("particle preset did not open in its actual editor: %+v, %v", presetDoc, err)
	}
	canvasID, presetPanelID := "render2d.art/"+artID.String(), "particles.preset/"+presetID.String()
	for range 8 {
		if !s.Step() {
			t.Fatal("editor stopped while laying out VFX")
		}
	}
	canvas, visible := ws.Tree.PanelRect(canvasID)
	if !visible || canvas.W < 300 || canvas.H < 220 {
		t.Fatalf("editable canvas is not visible beside the preset: %+v, %v", canvas, visible)
	}
	preview, visible := ws.Tree.PanelRect(presetPanelID)
	if !visible || preview.W < 120 || preview.H < 200 {
		t.Fatalf("particle preset panel is not visible beside the canvas: %+v, %v", preview, visible)
	}
	x, y := canvas.X+canvas.W/2, canvas.Y+canvas.H/2
	for i := range 3 {
		if !sdl.TestPointerEvent(x, y, i) || !s.Step() {
			t.Fatal("SDL could not draw in the VFX canvas")
		}
	}
	if model.History.Cursor() != 1 || !artDoc.Dirty() {
		t.Fatal("drawing in the VFX workspace did not create one dirty history entry")
	}
	if _, visible := ws.Tree.PanelRect(presetPanelID); !visible {
		t.Fatal("drawing hid the particle preset preview")
	}
	if !sdl.TestEditKey(input.KeyZ, true, true, false) || !s.Step() || !sdl.TestEditKey(input.KeyZ, false, false, false) || !s.Step() {
		t.Fatal("SDL could not undo the VFX drawing")
	}
	if model.History.Cursor() != 0 {
		t.Fatal("keyboard Undo did not reach the VFX art document")
	}
	// Capture the actual CPU particle preview at two points in its simulation.
	capture := func(name string) *image.RGBA {
		t.Helper()
		var result *image.RGBA
		var captureErr error
		s.Engine.CaptureFrame(func(img *image.RGBA, err error) { result, captureErr = img, err })
		if !s.Step() || captureErr != nil || result == nil {
			t.Fatalf("VFX workspace capture: %v", captureErr)
		}
		if dir := os.Getenv("AEGO_VFX_WORKSPACE_SHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			file, err := os.OpenFile(filepath.Join(dir, name+".png"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err != nil {
				t.Fatal(err)
			}
			writeErr := png.Encode(file, result)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				t.Fatalf("write VFX screenshot: %v, %v", writeErr, closeErr)
			}
		}
		return result
	}
	before := capture("preset-before")
	for range 30 {
		time.Sleep(15 * time.Millisecond)
		if !s.Step() {
			t.Fatal("editor stopped while previewing particles")
		}
	}
	after := capture("preset-after")
	changed := 0
	// Only the top preview square is compared; other editor regions cannot
	// accidentally satisfy this check through blinking cursors or layout changes.
	for yy := int(preview.Y + 5); yy < int(preview.Y+160) && yy < after.Bounds().Max.Y; yy++ {
		for xx := int(preview.X + 5); xx < int(preview.X+min(preview.W-5, 155)) && xx < after.Bounds().Max.X; xx++ {
			if before.RGBAAt(xx, yy) != after.RGBAAt(xx, yy) {
				changed++
			}
		}
	}
	if changed < 20 {
		t.Fatalf("particle preview did not visibly animate: %d changed pixels", changed)
	}
	// Change pixels in a saved frame, wait for the actual working generation,
	// then select this drawing and apply its existing frame identities using
	// the visible preset UI. No source is saved by the assignment.
	for i := range 3 {
		if !sdl.TestPointerEvent(x, y, i) || !s.Step() {
			t.Fatal("SDL could not draw before assigning the particle flipbook")
		}
	}
	for i := 0; i < 300 && !ed.WorkingSourceCurrent(artDoc, model.PreviewRevision()); i++ {
		time.Sleep(10 * time.Millisecond)
		if !s.Step() {
			t.Fatal("editor stopped while publishing the drawing preview")
		}
	}
	if !artDoc.Dirty() || !ed.WorkingSourceCurrent(artDoc, model.PreviewRevision()) {
		t.Fatalf("dirty drawing preview is not current: %+v", artDoc.WorkingPreview)
	}
	ui := ed.UI
	row, gap, pad := ui.Theme.RowHeight, ui.Theme.Spacing, ui.Theme.Padding
	contentW := preview.W - 2*pad - 9
	comboY := preview.Y + pad + 160 + gap + 3*(row+gap)
	if ed.OpenScript != nil {
		comboY += row + gap
	}
	comboX := preview.X + pad + contentW*.65
	if contentW < 230 {
		comboY += ui.LineHeight() + 2 + gap
	}
	click := func(px, py float32) {
		t.Helper()
		for i := range 3 {
			if !sdl.TestPointerEvent(px, py, i) || !s.Step() {
				t.Fatal("SDL could not operate the preset drawing picker")
			}
		}
	}
	click(comboX, comboY+row/2)
	if !ui.AnyPopup() {
		t.Fatalf("drawing picker did not open at %g,%g (panel %+v)", comboX, comboY+row/2, preview)
	}
	// The first popup item is the empty choice; the second is our drawing.
	click(comboX, comboY+row+pad+(row+gap)+row/2)
	hint := i18n.Tr("editor.particles.drawing_fps_hint", "Particles use constant FPS, not individual frame durations. Save new frames before assigning them. For multiple frames, zero FPS becomes 12.")
	assignY := comboY + row + gap + float32(len(widgets.WrapLines(ui, hint, contentW)))*(row+gap) + row/2
	beforeAssignment := bytes.Clone(presetDoc.Source.Code.Bytes())
	click(preview.X+preview.W/2, assignY)
	assigned, err := particles.ReadPreset(ed.Types, ed.Loader, presetDoc.Source.Code.Bytes())
	if err != nil || len(assigned.Frames) != 2 {
		notice, _ := ed.LastNotice()
		t.Fatalf("drawing picker did not assign the two frame IDs: %+v, %v; notice=%+v", assigned.Frames, err, notice)
	}
	for i, frame := range []art.ID{info.Frames[0].ID, second} {
		want, ok := ed.Pipeline.Lookup(artDoc.Path + "#" + artsource.FrameName(frame, art.Albedo))
		if !ok || assigned.Frames[i].ID != want {
			t.Fatalf("frame %d: got %s, want %s", i, assigned.Frames[i].ID, want)
		}
	}
	undoY := preview.Y + pad + 160 + gap + (row + gap) + row/2
	click(preview.X+preview.W/2, undoY)
	if !bytes.Equal(presetDoc.Source.Code.Bytes(), beforeAssignment) {
		t.Fatal("preset UI Undo did not restore the prior frames")
	}
	click(preview.X+preview.W/2, undoY+row+gap)
	assigned, err = particles.ReadPreset(ed.Types, ed.Loader, presetDoc.Source.Code.Bytes())
	if err != nil || len(assigned.Frames) != 2 {
		t.Fatalf("preset UI Redo lost the drawing frames: %v", err)
	}
	if disk, err := os.ReadFile(filepath.Join(p.Root, "assets", "workspace.aeart")); err != nil || !bytes.Equal(disk, source) {
		t.Fatalf("assigning frames changed the drawing on disk: %v", err)
	}
	for range 20 {
		time.Sleep(10 * time.Millisecond)
		if !s.Step() {
			t.Fatal("editor stopped after assigning the drawing")
		}
	}
	painted := capture("preset-with-drawing")
	red, green := 0, 0
	for yy := int(preview.Y + 5); yy < int(preview.Y+155) && yy < painted.Bounds().Max.Y; yy++ {
		for xx := int(preview.X + 5); xx < int(preview.X+min(preview.W-5, 155)) && xx < painted.Bounds().Max.X; xx++ {
			p := painted.RGBAAt(xx, yy)
			if p.R > 70 && int(p.R) > 2*int(p.G) {
				red++
			}
			if p.G > 45 && int(p.G) > 2*int(p.R) {
				green++
			}
		}
	}
	if red < 3 || green < 3 {
		t.Fatalf("particle preview did not show both authored frames: red=%d green=%d", red, green)
	}
	// A frame added only to the working document cannot be written as a durable
	// preset reference. The UI rejects it until the user saves the drawing.
	timeline, visible := ws.Tree.PanelRect("art.timeline")
	if !visible {
		t.Fatal("asset timeline is not visible in VFX")
	}
	click(timeline.X+pad+(timeline.W-2*pad)/8, timeline.Y+pad+2*(row+gap)+row/2)
	if len(model.Session.Document.Info().Frames) != 3 {
		t.Fatal("SDL did not add a frame through the asset timeline")
	}
	for i := 0; i < 300 && !ed.WorkingSourceCurrent(artDoc, model.PreviewRevision()); i++ {
		time.Sleep(10 * time.Millisecond)
		if !s.Step() {
			t.Fatal("editor stopped while previewing the new frame")
		}
	}
	if !ed.WorkingSourceCurrent(artDoc, model.PreviewRevision()) {
		t.Fatal("new-frame working preview did not publish")
	}
	beforeRejected := bytes.Clone(presetDoc.Source.Code.Bytes())
	click(preview.X+preview.W/2, assignY)
	notice, ok := ed.LastNotice()
	if !ok || !strings.Contains(notice.Text, "Save the drawing") || !bytes.Equal(beforeRejected, presetDoc.Source.Code.Bytes()) {
		t.Fatalf("unsaved frame was not safely rejected: notice=%+v", notice)
	}
	click(canvas.X+pad+row/2, canvas.Y+pad+row/2)
	if artDoc.Dirty() {
		t.Fatal("drawing did not save through its native toolbar")
	}
	var third art.ID
	for _, frame := range model.Session.Document.Info().Frames {
		if frame.ID != info.Frames[0].ID && frame.ID != second {
			third = frame.ID
		}
	}
	if third == "" {
		t.Fatal("new frame identity is missing")
	}
	for i := 0; i < 300; i++ {
		_, indexed := ed.Pipeline.Lookup(artDoc.Path + "#" + artsource.FrameName(third, art.Albedo))
		if indexed && ed.Pipeline.VerifyCurrent(artID) == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
		if !s.Step() {
			t.Fatal("editor stopped while importing the saved frame")
		}
	}
	click(preview.X+preview.W/2, assignY)
	assigned, err = particles.ReadPreset(ed.Types, ed.Loader, presetDoc.Source.Code.Bytes())
	if err != nil || len(assigned.Frames) != 3 {
		notice, _ := ed.LastNotice()
		t.Fatalf("saved third frame was not assignable: frames=%d err=%v notice=%+v", len(assigned.Frames), err, notice)
	}
	var savedFrames []ids.AssetID
	for _, frame := range assigned.Frames {
		savedFrames = append(savedFrames, frame.ID)
	}
	click(preview.X+preview.W/2, preview.Y+pad+160+gap+row/2)
	if presetDoc.Dirty() {
		notice, _ := ed.LastNotice()
		t.Fatalf("preset was not saved through its native UI: %+v", notice)
	}
	if err := ed.Pipeline.VerifyCurrent(presetID); err != nil {
		t.Fatalf("saving the preset did not update its imported asset: %v", err)
	}
	live, err := particles.LoadPreset(ed.Types, ed.Loader, presetID)
	if err != nil || len(live.Frames) != len(savedFrames) {
		t.Fatalf("saved preset is stale in the running editor: frames=%d err=%v", len(live.Frames), err)
	}
	s.Close()
	s = nil
	reopened, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1600, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatalf("reopen VFX project: %v", err)
	}
	defer reopened.Close()
	if !reopened.Step() {
		t.Fatal("reopened editor stopped")
	}
	reopenedEd := reopened.App.Context()
	reopenedID, ok := reopenedEd.Pipeline.Lookup("assets/workspace.aepreset")
	if !ok {
		t.Fatal("saved particle preset was not indexed after reopening")
	}
	loaded, err := particles.LoadPreset(reopenedEd.Types, reopenedEd.Loader, reopenedID)
	if err != nil || len(loaded.Frames) != len(savedFrames) {
		t.Fatalf("saved particle preset did not reopen: frames=%d err=%v", len(loaded.Frames), err)
	}
	for i, frame := range loaded.Frames {
		if frame.ID != savedFrames[i] || reopenedEd.Pipeline.VerifyCurrent(frame.ID) != nil {
			t.Fatalf("reopened frame %d is stale or changed identity: %s", i, frame.ID)
		}
	}
	cpu := particles.NewPreview(reopenedEd.Types, reopenedEd.Loader)
	defer cpu.Close()
	if err := cpu.Configure(loaded, 42); err != nil {
		t.Fatalf("reopened preset does not configure the CPU simulator: %v", err)
	}
	cpu.Burst(1)
	if cpu.Count() != 1 {
		t.Fatalf("reopened particle preset could not emit: %d particles", cpu.Count())
	}
}

// Exercises SDL/OpenGL Game View and CPU particles. Drawing and assignment use
// Go APIs, not the interactive VFX workspace.
func TestAuthoredFlipbookParticleGameViewNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	p := newProject(t)
	d, err := art.New(16, 16, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	info := d.Info()
	first := info.Frames[0].ID
	colors := []art.Color{
		{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255},
		{R: 255, G: 255, A: 255}, {R: 255, B: 255, A: 255},
		{G: 255, B: 255, A: 255}, {R: 255, G: 255, B: 255, A: 255},
		{R: 128, G: 128, B: 128, A: 255},
	}
	e, err := d.Begin(d.Revision(), "Draw particle frames")
	if err != nil {
		t.Fatal(err)
	}
	last := first
	for i, color := range colors {
		f := first
		if i != 0 {
			f, err = e.AddFrame(last, 100)
			if err != nil {
				t.Fatal(err)
			}
		}
		last = f
		cel, err := e.EnsureCel(info.Layers[0].ID, f, art.Albedo)
		if err != nil {
			t.Fatal(err)
		}
		for y := range 16 {
			for x := range 16 {
				if err := e.SetPixel(cel, x, y, art.Paint{Color: color}, nil, false); err != nil {
					t.Fatal(err)
				}
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
	frames := d.Info().Frames
	source, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	const artPath = "assets/fx.aeart"
	if err := os.WriteFile(filepath.Join(p.Root, artPath), source, 0644); err != nil {
		t.Fatal(err)
	}
	sourceID := ids.NewAssetID()
	imp := artsource.New()
	meta := asset.NewSourceMeta(imp.ID(), imp.Version(), imp.Defaults())
	meta.ID = sourceID
	if err := os.WriteFile(filepath.Join(p.Root, artPath+".meta"), asset.MarshalSourceMeta(meta), 0644); err != nil {
		t.Fatal(err)
	}
	var frameIDs []ids.AssetID
	var frameRefs []asset.Ref[asset.Texture]
	for _, frame := range frames {
		id := artsource.StableID(sourceID, artsource.FrameName(frame.ID, art.Albedo))
		frameIDs = append(frameIDs, id)
		frameRefs = append(frameRefs, asset.NewRef[asset.Texture](id))
	}
	const scenePath = "scenes/fx.aescene"
	writeScene(t, p, scenePath, func(sc *scene.Scene) {
		cam := sc.Create(sc.Root(), "Camera")
		*scene.Add[render2d.Camera2D](sc, cam) = render2d.DefaultCamera2D()
		h := sc.Create(sc.Root(), "Authored particle")
		c := scene.Add[particles.ParticleSystem2D](sc, h)
		*c = particles.DefaultParticleSystem2D()
		c.Frames = frameRefs
		c.FrameFPS, c.FrameLoop = 4, false
		c.Rate, c.PlayOnStart = 0, false
		c.Speed, c.Lifetime, c.Size = mathx.V2(0, 0), mathx.V2(5, 5), mathx.V2(2, 2)
		c.AlphaOverLife = curve.Constant(1)
	})
	cfg := Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()}
	s, err := Open(p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Engine.SetLockstep(true)
	if !s.Step() {
		t.Fatal("editor stopped at startup")
	}
	ed := s.App.Context()
	for _, id := range append([]ids.AssetID{sourceID}, frameIDs...) {
		if err := ed.Pipeline.VerifyCurrent(id); err != nil {
			t.Fatalf("authored asset %s is not current: %v", id, err)
		}
	}
	doc, err := ed.Docs.Open(scenePath)
	if err != nil {
		t.Fatal(err)
	}
	ws := s.App.Workspace()
	ws.Show(workspace.Game)
	ws.Focus(workspace.Game)
	if ws.Tree.Maximized() != workspace.Game && !ws.ToggleMaximize(workspace.Game) {
		t.Fatal("could not maximize Game View")
	}
	if err := s.App.Play().Start(doc); err != nil {
		t.Fatal(err)
	}
	defer s.App.Play().Stop()
	for range 4 {
		if !s.Step() {
			t.Fatal("editor stopped before particle burst")
		}
	}
	playScene := s.App.Play().Scene()
	h := playScene.Find(playScene.Root(), "Authored particle")
	fx := particles.Of(s.App.Play().World())
	if fx == nil || h.IsZero() {
		t.Fatal("actual particle world was not started")
	}
	fx.Burst(h, 1)
	if fx.Count(h) != 1 {
		t.Fatalf("actual CPU particle did not spawn: %+v", fx.Stats())
	}
	if !s.Step() {
		t.Fatal("editor stopped after particle burst")
	}
	s.App.Play().Pause()
	capture := func(name string) (image.Image, uint64) {
		t.Helper()
		for range 3 {
			if !s.Step() {
				t.Fatal("editor stopped before Game View capture")
			}
		}
		raw, tick, path, err := s.App.CaptureAgentView("game")
		if err != nil || path != scenePath {
			t.Fatalf("Game View capture: %s, %v", path, err)
		}
		img, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if dir := os.Getenv("AEGO_VFX_SHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			file, err := os.OpenFile(filepath.Join(dir, name+".png"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err != nil {
				t.Fatal(err)
			}
			_, writeErr := file.Write(raw)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				t.Fatalf("write Game View evidence: %v, %v", writeErr, closeErr)
			}
		}
		return img, tick
	}
	count := func(img image.Image, color art.Color) int {
		n := 0
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				close := func(v uint32, want byte) bool {
					delta := int(v>>8) - int(want)
					return delta >= -12 && delta <= 12
				}
				if close(r, color.R) && close(g, color.G) && close(b, color.B) {
					n++
				}
			}
		}
		return n
	}
	var previous image.Image
	var previousTick uint64
	for i, color := range colors {
		if i != 0 {
			s.App.Play().Pause()
			// At four frames per second, fifteen lockstep ticks advance one
			// authored frame. Capture is paused, so UI work cannot skip frames.
			for range 15 {
				if !s.Step() {
					t.Fatal("editor stopped while particle flipbook advanced")
				}
			}
			s.App.Play().Pause()
		}
		pixels, tick := capture(fmt.Sprintf("frame-%02d", i+1))
		if i != 0 && tick <= previousTick {
			t.Fatalf("particle simulation did not advance at frame %d: %d -> %d", i+1, previousTick, tick)
		}
		if visible := count(pixels, color); visible < 100 {
			t.Fatalf("Game View did not render authored frame %d (%s, %+v): %d matching pixels", i+1, frameIDs[i], color, visible)
		}
		if previous != nil {
			if previous.Bounds() != pixels.Bounds() {
				t.Fatal("Game View capture geometry changed during playback")
			}
			changed := 0
			for y := pixels.Bounds().Min.Y; y < pixels.Bounds().Max.Y; y++ {
				for x := pixels.Bounds().Min.X; x < pixels.Bounds().Max.X; x++ {
					r1, g1, b1, a1 := previous.At(x, y).RGBA()
					r2, g2, b2, a2 := pixels.At(x, y).RGBA()
					if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
						changed++
					}
				}
			}
			if changed < 100 {
				t.Fatalf("Game View did not visibly advance to frame %d: %d changed pixels", i+1, changed)
			}
		}
		previous, previousTick = pixels, tick
	}
}
