//go:build aego_gltest

package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"aego/app/agentui/backend"
	"aego/app/edit"
	"aego/app/edit/agentbridge"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/document"
	"aego/editor/core/preview"
	"aego/editor/core/workspace"
	"aego/gpu"
	"aego/modules/particles"
	"aego/project"
	"aego/scene"
	"aego/scene/sceneio"
)

func TestParticlePresetAuthoringMCPGL(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	root := copyExample(t, "platformer")
	proj, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(proj, edit.Config{Hidden: true, NoWatch: true, Width: 960, Height: 640, Logger: log.Nop()}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ed := s.App.Context()
	doc, err := ed.Docs.Open(bridgeScene)
	if err != nil {
		t.Fatal(err)
	}
	b, err := agentbridge.Start(s.AgentConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	e := &bridgeEnv{s: s, ctx: ed, doc: doc, b: b, root: root}
	tile := func(id ids.AssetID) ([]byte, string) {
		t.Helper()
		end := time.Now().Add(20 * time.Second)
		for time.Now().Before(end) {
			s.Step()
			im, rev, err := s.App.Previews().Content(id)
			if errors.Is(err, preview.ErrPending) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if why := s.App.Previews().Reason(id); why != "" {
				t.Fatal(why)
			}
			var buf bytes.Buffer
			if err := png.Encode(&buf, im); err != nil {
				t.Fatal(err)
			}
			return buf.Bytes(), rev
		}
		t.Fatal("thumbnail timed out")
		return nil, ""
	}
	hashes := map[[32]byte]string{}
	for _, name := range []string{"Dust", "Spark", "Smoke", "Rain", "Explosion"} {
		rel := "assets/presets/" + name + ".aepreset"
		id, ok := ed.Pipeline.Lookup(rel)
		if !ok {
			t.Fatal(rel)
		}
		raw, _ := tile(id)
		hash := sha256.Sum256(raw)
		if old := hashes[hash]; old != "" {
			t.Fatalf("%s and %s have identical previews", name, old)
		}
		hashes[hash] = name
		if dir := os.Getenv("AEGO_PARTICLE_TILES"); dir != "" {
			if err = os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, name+".png"), raw, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	const rel = "assets/presets/Smoke.aepreset"
	d, err := ed.OpenAssetPath(rel)
	if err != nil || d.Source == nil || d.Handler != "particles.preset" || !d.Capabilities.Has(document.EditSource) {
		t.Fatalf("preset routing: %+v %v", d, err)
	}
	again, err := ed.OpenAssetPath(rel)
	if err != nil || again != d {
		t.Fatal("opened duplicate preset document")
	}
	raw, rev := tile(d.Asset)
	src := d.Source.Code
	initial := bytes.Clone(src.Bytes())
	at := bytes.Index(initial, []byte("rate="))
	if at < 0 {
		t.Fatal("no smoke rate")
	}
	end := bytes.IndexByte(initial[at:], '\n')
	if end < 0 {
		t.Fatal("invalid preset")
	}
	changed := append(append(bytes.Clone(initial[:at]), []byte("rate=50")...), initial[at+end:]...)
	src.ReplaceSnapshot(changed, false)
	updated, newRev := tile(d.Asset)
	if rev == newRev || bytes.Equal(raw, updated) {
		t.Fatal("dirty preset didn't update thumbnail")
	}
	if doc.Dirty() {
		t.Fatal("preview dirtied the scene")
	}
	// Apply the unsaved preset through the shared proposal/Undo route.
	events := results(e.turn(t, []backend.Step{{Tool: "scene_add_comp", Args: map[string]any{"file": bridgeScene, "node": "BackdropStartLeft", "comp": "ParticleSystem2D", "preset": rel}}}, e.approveAll))
	if len(events) != 1 || events[0].Failed {
		t.Fatal(events)
	}
	sc := doc.Scene()
	node := sc.Find(sc.Root(), "BackdropStartLeft")
	c, ok := scene.Get[particles.ParticleSystem2D](sc, node)
	if !ok || c.Rate != 50 {
		t.Fatal("proposal did not use working preset")
	}
	if err = doc.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, ok = scene.Get[particles.ParticleSystem2D](sc, node); ok {
		t.Fatal("Undo retained added component")
	}
	if !src.Undo() || !bytes.Equal(src.Bytes(), initial) {
		t.Fatal("preset Undo changed another document")
	}
	src.ReplaceSnapshot(bytes.Replace(initial, []byte("maxParticles=20"), []byte("maxParticles=0"), 1), false)
	if err := ed.Docs.SaveAsset(d); err == nil {
		t.Fatal("invalid working preset was saved")
	}
	onDisk, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil || !bytes.Equal(onDisk, initial) {
		t.Fatal("invalid edit overwrote the preset on disk")
	}
	src.Undo()
	restored, _ := tile(d.Asset)
	if !bytes.Equal(raw, restored) {
		t.Fatal("Undo did not restore preview pixels")
	}

	client := backend.NewClient(b.Backend().MCPURL, b.Backend().Token)
	done := make(chan struct{})
	var result json.RawMessage
	go func() {
		result, err = client.CallRaw("asset_view", map[string]any{"asset": rel, "view": "preview"})
		close(done)
	}()
	deadline := time.After(12 * time.Second)
	for {
		select {
		case <-done:
			goto received
		case <-deadline:
			t.Fatal("MCP preview timed out")
		default:
			s.Step()
		}
	}
received:
	if err != nil {
		t.Fatal(err)
	}
	var rsp struct {
		IsError bool
		Content []struct{ Type, MimeType, Data string }
	}
	if err = json.Unmarshal(result, &rsp); err != nil || rsp.IsError {
		t.Fatalf("%v %s", err, result)
	}
	for _, item := range rsp.Content {
		if item.Type == "image" && item.MimeType == "image/png" {
			p, err := base64.StdEncoding.DecodeString(item.Data)
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(p))
			if err != nil || img.Bounds().Empty() {
				t.Fatal("bad MCP image", err)
			}
			if err := ed.Docs.CloseAsset(d.Asset, false); err != nil {
				t.Fatal(err)
			}
			if d.View != nil || s.App.Workspace().Has("particles.preset/"+d.Asset.String()) || doc.Dirty() {
				t.Fatal("closing preview kept the tab or dirtied the scene")
			}
			return
		}
	}
	t.Fatal("provider transport returned no PNG image block")
}

// Reopen each authored preset through its document route; after warmup each cycle must retire the preview world, panel and GPU resources without changing the scene, asset source or undo cursor.
func TestParticlePresetThirtyEditorCyclesGL(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	root := copyExample(t, "platformer")
	proj, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(proj, edit.Config{Hidden: true, NoWatch: true, Width: 960, Height: 640, Logger: log.Nop()}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ed := s.App.Context()
	doc, err := ed.Docs.Open(bridgeScene)
	if err != nil {
		t.Fatal(err)
	}
	cursor := doc.History.Cursor()
	reporter, ok := ed.Renderer.Device().(gpu.ResourceReporter)
	if !ok {
		t.Fatal("resource telemetry missing")
	}
	var baseline gpu.ResourceStats
	for pass := 0; pass <= 30; pass++ {
		for _, name := range []string{"Dust", "Spark", "Smoke", "Rain", "Explosion"} {
			d, err := ed.OpenAssetPath("assets/presets/" + name + ".aepreset")
			if err != nil || d.Handler != "particles.preset" || d.Source == nil {
				t.Fatalf("%s: open: %v", name, err)
			}
			if !s.Step() || !s.Step() {
				t.Fatal("editor stopped")
			}
			if d.Dirty() {
				t.Fatalf("%s preview dirtied the preset", name)
			}
			if err = ed.Docs.CloseAsset(d.Asset, false); err != nil {
				t.Fatal(err)
			}
			if d.View != nil || s.App.Workspace().Has("particles.preset/"+d.Asset.String()) {
				t.Fatal("closed preview panel remains alive", name)
			}
		}
		for range 5 {
			s.Step()
		} // drain the renderer's normal retirement window
		got := reporter.ResourceStats()
		if pass == 0 {
			baseline = got
			continue
		}
		if got.Buffers > baseline.Buffers || got.Textures > baseline.Textures || got.Framebuffers > baseline.Framebuffers || got.Shaders > baseline.Shaders || got.Pipelines > baseline.Pipelines || got.PendingCompiles != 0 || got.TextureStorageBytes > baseline.TextureStorageBytes {
			t.Fatalf("closed preview resources grew on cycle %d: from %+v to %+v", pass, baseline, got)
		}
	}
	if doc.Dirty() || doc.History.Cursor() != cursor {
		t.Fatal("preview changed authored scene")
	}
	t.Logf("30 cycles × 5 presets; resources after warmup: %+v, after final close: %+v", baseline, reporter.ResourceStats())
}

func TestPlatformerThirtyPlayStopCyclesGL(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	root := copyExample(t, "platformer")
	proj, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(proj, edit.Config{Hidden: true, NoWatch: true, Width: 800, Height: 600, Logger: log.Nop()}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ed := s.App.Context()
	doc, err := ed.Docs.Open(bridgeScene)
	if err != nil {
		t.Fatal(err)
	}
	s.App.Workspace().Show(workspace.Game)
	s.App.Workspace().Focus(workspace.Game)
	cursor := doc.History.Cursor()
	startDoc := sceneio.Save(doc.Scene())
	reporter, ok := ed.Renderer.Device().(gpu.ResourceReporter)
	if !ok {
		t.Fatal("no GPU resource telemetry")
	}
	var baseline gpu.ResourceStats
	pc := s.App.Play()
	for cycle := 0; cycle <= 30; cycle++ {
		if err := pc.Start(doc); err != nil {
			t.Fatal(err)
		}
		for range 5 {
			if !s.Step() {
				t.Fatal("editor stopped")
			}
		}
		if ed.Renderer.Stats().PostFXPasses == 0 {
			t.Fatal("Play did not exercise the camera output")
		}
		pc.Stop()
		for range 5 {
			s.Step()
		}
		if pc.World() != nil || pc.Scene() != nil || doc.Dirty() || doc.History.Cursor() != cursor {
			t.Fatal("play world or authoring state survived Stop")
		}
		got := reporter.ResourceStats()
		if cycle == 0 {
			baseline = got
			continue
		}
		if got.Buffers > baseline.Buffers || got.Textures > baseline.Textures || got.Samplers > baseline.Samplers || got.Shaders > baseline.Shaders || got.Pipelines > baseline.Pipelines || got.Framebuffers > baseline.Framebuffers || got.TextureStorageBytes > baseline.TextureStorageBytes || got.PendingCompiles != 0 {
			t.Fatalf("cycle %d retained graphics resources: warm %+v, now %+v", cycle, baseline, got)
		}
	}
	if !bytes.Equal(startDoc, sceneio.Save(doc.Scene())) {
		t.Fatal("Play/Stop changed the source scene")
	}
	t.Logf("30 Play/Stop cycles; warm %+v, final %+v", baseline, reporter.ResourceStats())
}
